import fileinput

import sys
import os
import re
import subprocess

def processFile(env,yaml):
    regex = "export [^=\n]+=[^\n]+\n"
    isTheLine = False
    fyaml = open(yaml,'w+')
    isrunner = False
    for line in fileinput.input(env, inplace=True): 
        if isrunner:
            fyaml.write(line)
            continue
        if '### Runner' in line:
            isrunner = True
            continue
        if line == '\n':
            sys.stdout.write (line)
            continue
        m = re.search(regex,line)
        if m:
            sys.stdout.write(m.group(0)[7:])
        elif line.startswith('#'):
            sys.stdout.write(line)
        else:
            fyaml.write(line)


def processYaml(yaml, envname):
    print "process yaml!"

# set -o errexit
# set -o nounset
# set -o pipefail
# set -o xtrace
# readonly testinfra="$(dirname "${0}")/.."
# readonly runner="${testinfra}/jenkins/dockerized-e2e-runner.sh"
# export DOCKER_TIMEOUT="200m"
# export KUBEKINS_TIMEOUT="180m"
# "${runner}"

    for line in fileinput.input(yaml, inplace=True):
        if "set -o" in line:
            # skip
            continue
        elif "readonly testinfra=" in line:
            # don't need for now
            continue
        elif "readonly runner" in line:
            reg = "readonly runner=\"([^\"]+)\""
            m = re.search(reg,line)
            if m:
                sys.stdout.write("runner: ")
                sys.stdout.write(m.group(1))
                sys.stdout.write("\n")
        elif "DOCKER_TIMEOUT" in line:
            reg = "export DOCKER_TIMEOUT=\"([0-9]+)m\""
            m = re.search(reg,line)
            if m:
                sys.stdout.write("docker_timeout: ")
                sys.stdout.write(m.group(1))
                sys.stdout.write("\n")
        elif "KUBEKINS_TIMEOUT" in line:
            reg = "export KUBEKINS_TIMEOUT=\"([0-9])+m\""
            m = re.search(reg,line)
            if m:
                sys.stdout.write("kubekins_timeout: ")
                sys.stdout.write(m.group(1))
                sys.stdout.write("\n")
        elif "\"${runner}\"" in line:
            sys.stdout.write("env: ")
            sys.stdout.write(envname + "\n")


def processEnv(env):
    print "process env!"

    with open(env) as fp:
        envfile = fp.read()
    m = re.search(r"E2E_NAME='([^']+)'", envfile)
    if not m:
        m = re.search(r"E2E_NAME=\"([^\"]+)\"", envfile)
    if not m:
        print "ERROR cannot find E2E_NAME from " + env
        return

    e2e_name = m.group(1)
    
    for line in fileinput.input(env, inplace=True):
        if "\"${E2E_NAME}\"" in line and e2e_name:
            line = line.replace("\"${E2E_NAME}\"", '\'' + e2e_name + '\'')
        m = re.search(r"([0-9a-zA-Z_]+)=\"\${[^:]+:-([\w]+)}\"", line)
        if m:
            name = m.group(1)
            val = m.group(2)
            line = name + '=' + val + '\n'
        sys.stdout.write(line)


if __name__ == '__main__':
    indir = '.'
    for root, dirs, filenames in os.walk(indir):
        for f in filenames:
            print "%s - %s" % (root,f)
            if "ci-kubernetes-e2e" not in f:
                continue
            if ".sh" not in f:
                continue

            print "Start Conversion"
            env = f.replace(".sh",".env")
            yaml = f.replace(".sh",".yaml")
            shpath = root + "/" + f
            envpath = root + "/" + env
            yamlpath = root + "/" + yaml

            # sh -> env
            subprocess.check_call(['git','mv',shpath,envpath])
            # subprocess.check_call(['mv',shpath,envpath])

            print "process file"
            processFile(envpath, yamlpath)
            processYaml(yamlpath, f)
            processEnv(envpath)

