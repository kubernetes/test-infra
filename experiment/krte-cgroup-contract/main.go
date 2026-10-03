/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Command krte-cgroup-contract records the CPU cgroup topology a process sees.
// It collects best-effort: parse problems become entries in "errors" and the
// snapshot is still written, so the most interesting cases (a hidden ancestor,
// a virtualized mount root, an unexpected placement) leave evidence instead of
// an empty artifact. Assertions belong to the caller, not here.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

type cgroupMount struct {
	version int
	root    string
	point   string
	options []string // super options; v1 controllers appear here
}

type cpuLevel struct {
	Path                string `json:"path"`
	Type                string `json:"type,omitempty"`
	CPUMax              string `json:"cpu_max,omitempty"`
	CPUStat             string `json:"cpu_stat,omitempty"`
	CPUSetCPUsEffective string `json:"cpuset_cpus_effective,omitempty"`
}

type snapshot struct {
	Label             string            `json:"label"`
	PID               int               `json:"pid"`
	Runtime           string            `json:"runtime"`
	NumCPU            int               `json:"num_cpu"`
	GOMAXPROCS        int               `json:"gomaxprocs"`
	GODEBUG           string            `json:"godebug,omitempty"`
	GOMAXPROCSEnv     string            `json:"gomaxprocs_env,omitempty"`
	CgroupMode        string            `json:"cgroup_mode"` // unified|hybrid|legacy|unknown
	UnsupportedReason string            `json:"unsupported_reason,omitempty"`
	RawCgroup         string            `json:"raw_cgroup,omitempty"`
	CgroupV2Path      string            `json:"cgroup_v2_path,omitempty"`
	MountRoot         string            `json:"mount_root,omitempty"`
	MountPoint        string            `json:"mount_point,omitempty"`
	NamespaceLinks    map[string]string `json:"namespace_links,omitempty"`
	// MinimumVisibleCPUQuota is the smallest quota/period found in the visible
	// chain. It is a quota, not an effective core count: it ignores affinity,
	// cpuset.cpus.effective, NumCPU, and Go's rounding.
	MinimumVisibleCPUQuota float64    `json:"minimum_visible_cpu_quota,omitempty"`
	HasVisibleCPULimit     bool       `json:"has_visible_cpu_limit"`
	Levels                 []cpuLevel `json:"levels,omitempty"`
	Errors                 []string   `json:"errors,omitempty"`
}

func (s *snapshot) addErr(format string, a ...any) {
	s.Errors = append(s.Errors, fmt.Sprintf(format, a...))
}

func main() {
	label := flag.String("label", "process", "snapshot label")
	output := flag.String("output", "", "also write JSON to this file")
	hold := flag.Duration("hold", 0, "stay alive after writing the snapshot")
	summarize := flag.String("summarize", "", "read outer.json and docker-child.json from this directory and write summary.json")
	flag.Parse()

	if *summarize != "" {
		if err := writeSummary(*summarize); err != nil {
			fmt.Fprintf(os.Stderr, "cgroup contract: summarize: %v\n", err)
			os.Exit(1)
		}
		return
	}

	s := takeSnapshot(*label)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cgroup contract: encode snapshot: %v\n", err)
		os.Exit(1)
	}
	data = append(data, '\n')

	// Write the artifact before stdout so a broken pipe cannot lose it.
	if *output != "" {
		if err := os.WriteFile(*output, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "cgroup contract: write %s: %v\n", *output, err)
			os.Exit(1)
		}
	}
	os.Stdout.Write(data)
	if *hold > 0 {
		time.Sleep(*hold)
	}
}

func takeSnapshot(label string) snapshot {
	s := snapshot{
		Label:         label,
		PID:           os.Getpid(),
		Runtime:       runtime.Version(),
		NumCPU:        runtime.NumCPU(),
		GOMAXPROCS:    runtime.GOMAXPROCS(0),
		GODEBUG:       os.Getenv("GODEBUG"),
		GOMAXPROCSEnv: os.Getenv("GOMAXPROCS"),
		CgroupMode:    "unknown",
	}
	s.readNamespaceLinks()

	raw, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		s.addErr("read /proc/self/cgroup: %v", err)
		return s
	}
	s.RawCgroup = strings.TrimSpace(string(raw))
	v2path, v1, err := parseProcCgroup(strings.NewReader(s.RawCgroup))
	if err != nil {
		s.addErr("parse /proc/self/cgroup: %v", err)
	}
	_, cpuOnV1 := v1["cpu"]
	switch {
	case v2path != "" && len(v1) == 0:
		s.CgroupMode = "unified"
	case v2path != "" && len(v1) > 0:
		s.CgroupMode = "hybrid"
	case v2path == "" && len(v1) > 0:
		s.CgroupMode = "legacy"
	}

	mountData, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		s.addErr("read /proc/self/mountinfo: %v", err)
		return s
	}
	mounts, err := parseMounts(strings.NewReader(string(mountData)))
	if err != nil {
		s.addErr("parse mountinfo: %v", err)
	}

	if cpuOnV1 || v2path == "" {
		s.readV1CPU(v1["cpu"], mounts)
		return s
	}
	s.CgroupV2Path = v2path
	s.readV2Chain(v2path, mounts)
	return s
}

func (s *snapshot) readNamespaceLinks() {
	links := map[string]string{}
	for _, ns := range []string{"cgroup", "mnt", "pid"} {
		link, err := os.Readlink("/proc/self/ns/" + ns)
		if err != nil {
			s.addErr("readlink ns/%s: %v", ns, err)
			continue
		}
		links[ns] = link
	}
	if len(links) > 0 {
		s.NamespaceLinks = links
	}
}

// readV2Chain walks the unified leaf up to the mount point, reading cpu.max at
// each level, and records the smallest quota it can see.
func (s *snapshot) readV2Chain(v2path string, mounts []cgroupMount) {
	m, ok := pickVisibleV2Mount(v2path, mounts)
	if !ok {
		s.UnsupportedReason = "no visible cgroup2 mount contains the current path"
		return
	}
	s.MountRoot, s.MountPoint = m.root, m.point
	leaf, err := fullLeaf(v2path, m)
	if err != nil {
		s.addErr("resolve leaf: %v", err)
		return
	}

	var limits []float64
	for path := leaf; ; path = filepath.Dir(path) {
		l := cpuLevel{
			Path:                path,
			Type:                readOptional(filepath.Join(path, "cgroup.type")),
			CPUMax:              readOptional(filepath.Join(path, "cpu.max")),
			CPUStat:             readOptional(filepath.Join(path, "cpu.stat")),
			CPUSetCPUsEffective: readOptional(filepath.Join(path, "cpuset.cpus.effective")),
		}
		s.Levels = append(s.Levels, l)
		if value, ok, err := parseCPUMax(l.CPUMax); err != nil {
			s.addErr("parse %s/cpu.max: %v", path, err)
		} else if ok {
			limits = append(limits, value)
		}
		if path == m.point {
			break
		}
		if parent := filepath.Dir(path); parent == path || !within(parent, m.point) {
			s.addErr("cgroup walk escaped mount point %q at %q", m.point, path)
			break
		}
	}
	if len(limits) > 0 {
		s.HasVisibleCPULimit = true
		s.MinimumVisibleCPUQuota = slices.Min(limits)
	}
}

// readV1CPU reads cpu.cfs_quota_us / cpu.cfs_period_us for the v1 cpu hierarchy.
func (s *snapshot) readV1CPU(cpuPath string, mounts []cgroupMount) {
	if cpuPath == "" {
		s.UnsupportedReason = "cpu controller not found in /proc/self/cgroup"
		return
	}
	var mount *cgroupMount
	for i := range mounts {
		if mounts[i].version == 1 && slices.Contains(mounts[i].options, "cpu") {
			mount = &mounts[i]
			break
		}
	}
	if mount == nil {
		s.UnsupportedReason = "cpu controller is on cgroup v1 but no v1 cpu mount is visible"
		return
	}
	s.MountRoot, s.MountPoint = mount.root, mount.point
	leaf, err := fullLeaf(cpuPath, *mount)
	if err != nil {
		s.addErr("resolve v1 cpu leaf: %v", err)
		return
	}
	quotaRaw := readOptional(filepath.Join(leaf, "cpu.cfs_quota_us"))
	periodRaw := readOptional(filepath.Join(leaf, "cpu.cfs_period_us"))
	s.Levels = append(s.Levels, cpuLevel{Path: leaf, CPUMax: quotaRaw + " " + periodRaw})
	quota, err := strconv.ParseFloat(quotaRaw, 64)
	if err != nil || quota < 0 {
		return // -1 means unlimited; anything unparsable is left as no limit
	}
	period, err := strconv.ParseFloat(periodRaw, 64)
	if err != nil || period <= 0 {
		s.addErr("invalid v1 cpu.cfs_period_us %q", periodRaw)
		return
	}
	s.HasVisibleCPULimit = true
	s.MinimumVisibleCPUQuota = quota / period
	// v1 here reads only the leaf, unlike the v2 chain that walks to the mount
	// point, so this value is not the visible-ancestor minimum the field name
	// implies on v2. Flag the gap rather than let it read as the same guarantee.
	s.UnsupportedReason = "v1 cpu quota is read leaf-only; the visible-ancestor minimum is not computed"
}

// parseProcCgroup returns the unified v2 path (hierarchy 0) and a
// controller->path map for any v1 hierarchies.
func parseProcCgroup(r io.Reader) (v2path string, v1 map[string]string, err error) {
	v1 = map[string]string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), ":", 3)
		if len(fields) != 3 {
			continue
		}
		switch {
		case fields[0] == "0" && fields[1] == "":
			if !strings.HasPrefix(fields[2], "/") {
				return "", nil, fmt.Errorf("malformed cgroup v2 path %q", fields[2])
			}
			v2path = filepath.Clean(fields[2])
		case fields[1] != "":
			for _, c := range strings.Split(fields[1], ",") {
				v1[c] = fields[2]
			}
		}
	}
	return v2path, v1, scanner.Err()
}

// parseMounts returns every cgroup and cgroup2 mount from a mountinfo stream.
func parseMounts(r io.Reader) ([]cgroupMount, error) {
	var mounts []cgroupMount
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		sep := slices.Index(fields, "-")
		// after "-": fstype (sep+1), source (sep+2), super options (sep+3)
		if sep < 5 || sep+3 >= len(fields) {
			continue
		}
		var m cgroupMount
		switch fields[sep+1] {
		case "cgroup2":
			m.version = 2
		case "cgroup":
			m.version = 1
		default:
			continue
		}
		m.root = filepath.Clean(unescapeMount(fields[3]))
		m.point = filepath.Clean(unescapeMount(fields[4]))
		m.options = strings.Split(fields[sep+3], ",")
		mounts = append(mounts, m)
	}
	return mounts, scanner.Err()
}

// pickVisibleV2Mount returns the cgroup2 mount whose root is the longest prefix
// of cgroup, i.e. the one that actually contains it.
func pickVisibleV2Mount(cgroup string, mounts []cgroupMount) (cgroupMount, bool) {
	best := cgroupMount{}
	bestLen := -1
	for _, m := range mounts {
		if m.version != 2 {
			continue
		}
		if m.root != "/" && cgroup != m.root && !strings.HasPrefix(cgroup, m.root+"/") {
			continue
		}
		if len(m.root) > bestLen {
			best, bestLen = m, len(m.root)
		}
	}
	return best, bestLen >= 0
}

func fullLeaf(cgroup string, m cgroupMount) (string, error) {
	var relative string
	switch {
	case m.root == "/":
		relative = cgroup
	case cgroup == m.root:
		relative = "/"
	default:
		rest, ok := strings.CutPrefix(cgroup, m.root+"/")
		if !ok {
			return "", fmt.Errorf("cgroup %q is outside mount root %q", cgroup, m.root)
		}
		relative = "/" + rest
	}
	return filepath.Join(m.point, relative), nil
}

func within(path, point string) bool {
	rel, err := filepath.Rel(point, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

func parseCPUMax(raw string) (float64, bool, error) {
	if raw == "" || raw == "<absent>" {
		return 0, false, nil
	}
	quotaStr, periodStr, ok := strings.Cut(raw, " ")
	if !ok {
		return 0, false, fmt.Errorf("malformed value %q", raw)
	}
	if quotaStr == "max" {
		return 0, false, nil
	}
	quota, err := strconv.ParseFloat(quotaStr, 64)
	if err != nil {
		return 0, false, err
	}
	period, err := strconv.ParseFloat(periodStr, 64)
	if err != nil || period <= 0 {
		return 0, false, fmt.Errorf("invalid period %q", periodStr)
	}
	return quota / period, true, nil
}

func readOptional(path string) string {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "<absent>"
	}
	if err != nil {
		return "<error: " + err.Error() + ">"
	}
	return strings.TrimSpace(string(data))
}

func unescapeMount(s string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}

type summaryEntry struct {
	CgroupMode             string  `json:"cgroup_mode,omitempty"`
	CgroupV2Path           string  `json:"cgroup_v2_path,omitempty"`
	MinimumVisibleCPUQuota float64 `json:"minimum_visible_cpu_quota,omitempty"`
	HasVisibleCPULimit     bool    `json:"has_visible_cpu_limit"`
	GOMAXPROCS             int     `json:"gomaxprocs"`
	OuterNamespaceCgroup   string  `json:"outer_namespace_cgroup,omitempty"`
	UnsupportedReason      string  `json:"unsupported_reason,omitempty"`
	Errors                 int     `json:"errors,omitempty"`
}

type summaryDoc struct {
	Provenance         map[string]string `json:"provenance,omitempty"`
	OuterDefault       *summaryEntry     `json:"outer_default,omitempty"`
	OuterForced        *summaryEntry     `json:"outer_forced,omitempty"`
	DockerChildDefault *summaryEntry     `json:"docker_child_default,omitempty"`
	DockerChildForced  *summaryEntry     `json:"docker_child_forced,omitempty"`
	KindNode           *summaryEntry     `json:"kind_node,omitempty"`
}

// writeSummary reduces every layer that produced a snapshot to the compared
// fields, alongside the run's provenance. A layer that did not run is omitted,
// so the summary reports the layers that actually ran rather than implying all
// of them did.
func writeSummary(dir string) error {
	hostCgroup := ""
	if data, err := os.ReadFile(filepath.Join(dir, "docker-child-from-outer.cgroup")); err == nil {
		hostCgroup = strings.TrimSpace(string(data))
	}
	doc := summaryDoc{
		Provenance:         readProvenance(dir),
		OuterDefault:       entryFromFile(dir, "outer-default.json", ""),
		OuterForced:        entryFromFile(dir, "outer.json", ""),
		DockerChildDefault: entryFromFile(dir, "docker-child-default.json", ""),
		DockerChildForced:  entryFromFile(dir, "docker-child.json", hostCgroup),
		KindNode:           entryFromFile(dir, "kind-node-go.json", ""),
	}
	if doc.OuterForced == nil {
		return fmt.Errorf("outer snapshot missing; nothing to summarize")
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), data, 0o644); err != nil {
		return err
	}
	os.Stdout.Write(data)
	return nil
}

// entryFromFile reads one snapshot file into a summaryEntry, returning nil when
// the file is absent so a layer that did not run is left out of the summary.
func entryFromFile(dir, name, hostCgroup string) *summaryEntry {
	var s snapshot
	if err := readSnapshot(filepath.Join(dir, name), &s); err != nil {
		return nil
	}
	e := entryOf(s, hostCgroup)
	return &e
}

// readProvenance parses the key=value lines verify.sh writes to provenance.txt.
func readProvenance(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, "provenance.txt"))
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func entryOf(s snapshot, hostCgroup string) summaryEntry {
	return summaryEntry{
		CgroupMode:             s.CgroupMode,
		CgroupV2Path:           s.CgroupV2Path,
		MinimumVisibleCPUQuota: s.MinimumVisibleCPUQuota,
		HasVisibleCPULimit:     s.HasVisibleCPULimit,
		GOMAXPROCS:             s.GOMAXPROCS,
		OuterNamespaceCgroup:   hostCgroup,
		UnsupportedReason:      s.UnsupportedReason,
		Errors:                 len(s.Errors),
	}
}

func readSnapshot(path string, s *snapshot) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, s)
}
