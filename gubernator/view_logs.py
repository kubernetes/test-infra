# Copyright 2016 The Kubernetes Authors All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os

import gcs_async
import log_parser
import kubelet_parser
import regex
from view_base import BaseHandler, memcache_memoize, gcs_ls


@memcache_memoize('log-file-junit://', expires=60*60*4)
def find_log_junit((build_dir, junit, log_file)):
    '''
    Looks in build_dir for log_file in a folder that
    also includes the junit file.
    '''
    tmps = [f.filename for f in gcs_ls('%s/artifacts' % build_dir)
            if '/tmp-node' in f.filename]
    for folder in tmps:
        filenames = [f.filename for f in gcs_ls(folder)]
        if folder + junit in filenames:
            path = folder + log_file
            if path in filenames:
                return path


def find_log_files(all_logs, log_file):
    '''
    Returns list of files named log_file from values in all_logs
    '''
    log_files = []
    for folder in all_logs.itervalues():
        for log in folder:
            if log_file in log:
                log_files.append(log)

    return log_files


@memcache_memoize('all-logs://', expires=60*60*4)
def get_all_logs((directory, artifacts)):
    '''
    returns dictionary given the artifacts folder with the keys being the
    folders, and the values being the log files within the corresponding folder
    '''
    log_files = {}
    if artifacts:
        dirs = [f.filename for f in gcs_ls('%s/artifacts' % directory)
                if f.is_dir]
    else:
        dirs = [directory]
    for d in dirs:
        log_files[d] = []
        for f in gcs_ls(d):
            log_name = regex.log_re.search(f.filename)
            if log_name:
                log_files[d].append(f.filename)
    return log_files


def parse_log_file(log_filename, pod, filters=None, make_dict=False, objref_dict=None):
    """Based on make_dict, either returns the objref_dict or the parsed log file"""
    log = gcs_async.read(log_filename).get_result()
    if log is None:
        return {}, False if make_dict else None
    pod_re = regex.wordRE(pod)
    if objref_dict is None:
        objref_dict = {}
    if make_dict:
        return kubelet_parser.make_dict(log.decode('utf8', 'replace'), pod_re, objref_dict)
    else:
        return log_parser.digest(log.decode('utf8', 'replace'),
        error_re=pod_re, filters=filters, objref_dict=objref_dict)


def get_logs_junit((log_files, pod_name, filters, objref_dict, apiserver_filename)):
    # Get the logs in the case where the junit file with the failure is in a specific folder
    all_logs = {}
    results = {}

    # default to filtering kube-apiserver log if user unchecks both checkboxes
    if log_files == []:
        log_files = [apiserver_filename]

    artifact_filename = os.path.dirname(apiserver_filename)
    all_logs = get_all_logs((artifact_filename, False))
    parsed_dict, _ = parse_log_file(os.path.join(artifact_filename, "kubelet.log"),
        pod_name, make_dict=True, objref_dict=objref_dict)
    objref_dict.update(parsed_dict)
    if log_files:
        for log_file in log_files:
            parsed_file = parse_log_file(log_file, pod_name, filters, objref_dict=objref_dict)
            if parsed_file:
                results[log_file] = parsed_file

    return all_logs, results, objref_dict, log_files


def get_logs_no_pod(apiserver_filename, kubelet_filenames, filters, objref_dict, all_logs):
    # Get results of parsing logs when no pod name is given
    results = {}
    if apiserver_filename:
        for apiserver_log in apiserver_filename:
            parsed_file = parse_log_file(apiserver_log, "", filters,
            objref_dict=objref_dict)
            if parsed_file:
                results[apiserver_log] = parsed_file
        return all_logs, results, objref_dict, apiserver_filename
    else:
        for kubelet_log in kubelet_filenames:
            parsed_file = parse_log_file(kubelet_log, "", filters,
            objref_dict=objref_dict)
            if parsed_file:
                results[kubelet_log] = parsed_file
        return all_logs, results, objref_dict, kubelet_filenames


def get_logs(build_dir, log_files, pod_name, filters, objref_dict):
    """
    Get the logs in the case where all logs in artifacts folder may be relevant
    Returns:
        all_logs: dictionary of all logs that can be filtered
        results: dictionary of log file to the parsed text
        obref_dict: dictionary of name of filter to the string to be filtered
        log_files: list of files that are being displayed/filtered
    """
    all_logs = {}
    results = {}
    old_dict_len = len(objref_dict)

    all_logs = get_all_logs((build_dir, True))
    apiserver_filename = find_log_files(all_logs, "kube-apiserver.log")
    kubelet_filenames = find_log_files(all_logs, "kubelet.log")
    if not pod_name and not objref_dict:
        return get_logs_no_pod(apiserver_filename, kubelet_filenames, filters,
            objref_dict, all_logs)
    for kubelet_log in kubelet_filenames:
        if pod_name:
            parsed_dict, pod_in_file = parse_log_file(kubelet_log, pod_name, make_dict=True,
                objref_dict=objref_dict)
            objref_dict.update(parsed_dict)
        if len(objref_dict) > old_dict_len or not pod_name or pod_in_file:
            if log_files == []:
                log_files = [kubelet_log]
                if apiserver_filename:
                    log_files.extend(apiserver_filename)
            for log_file in log_files:
                parsed_file = parse_log_file(log_file, pod_name, filters,
                    objref_dict=objref_dict)
                if parsed_file:
                    results[log_file] = parsed_file
            break

    return all_logs, results, objref_dict, log_files


class NodeLogHandler(BaseHandler):
    def get(self, prefix, job, build):
        """
        Examples of variables
        log_files: ["kubelet.log", "kube-apiserver.log"]
        pod_name: "pod-abcdef123"
        junit: "junit_01.xml"
        uid, namespace, wrap: "on"
        cID, poduid, ns: strings entered into textboxes
        results, logs: {"kubelet.log":"parsed kubelet log for html"}
        all_logs: {"folder_name":["a.log", "b.log"]}
        """
        # pylint: disable=too-many-locals
        self.check_bucket(prefix)
        job_dir = '/%s/%s/' % (prefix, job)
        build_dir = job_dir + build
        log_files = self.request.get_all("logfiles")
        pod_name = self.request.get("pod")
        junit = self.request.get("junit")
        cID = self.request.get("cID")
        poduid = self.request.get("poduid")
        ns = self.request.get("ns")
        uid = bool(self.request.get("UID"))
        namespace = bool(self.request.get("Namespace"))
        containerID = bool(self.request.get("ContainerID"))
        wrap = bool(self.request.get("wrap"))
        filters = {"UID":uid, "pod":pod_name, "Namespace":namespace, "ContainerID":containerID}
        objref_dict = {}
        results = {}

        if cID:
            objref_dict["ContainerID"] = cID
        if poduid:
            objref_dict["UID"] = poduid
        if ns:
            objref_dict["Namespace"] = ns

        apiserver_filename = find_log_junit((build_dir, junit, "kube-apiserver.log"))
        if apiserver_filename and pod_name:
            all_logs, results, objref_dict, log_files = get_logs_junit((log_files,
                pod_name, filters, objref_dict, apiserver_filename))
        if not apiserver_filename:
            all_logs, results, objref_dict, log_files = get_logs(build_dir, log_files,
                pod_name, filters, objref_dict)

        if results == {}:
            self.render('node_404.html', {"build_dir": build_dir, "log_files": log_files,
                "pod_name":pod_name, "junit":junit})
            self.response.set_status(404)
            return

        self.render('filtered_log.html', dict(
            job_dir=job_dir, build_dir=build_dir, logs=results, job=job,
            build=build, log_files=log_files, containerID=containerID,
            pod=pod_name, junit=junit, uid=uid, namespace=namespace,
            wrap=wrap, objref_dict=objref_dict, all_logs=all_logs))
