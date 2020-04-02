package common

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	defTmpDir = "/tmp"
)

//RunShellCmdWithEnv is a function to run a shell command with new os environament variable and special dir
func RunShellCmdWithEnv(env []string, workingDir, script string) (string, error) {
	tmpDir, err := ioutil.TempDir(workingDir, "hive")
	if err != nil {
		fmt.Println("Create Tmp Dir error:", err)
		return "", err
	}

	defer os.RemoveAll(tmpDir) // clean up
	if workingDir == defTmpDir {
		// run in the tmpDir
		workingDir = tmpDir
	}

	scriptFile := filepath.Join(workingDir, "scriptFileile")
	defer os.Remove(scriptFile)
	if err := ioutil.WriteFile(scriptFile, []byte(script), 0666); err != nil {
		fmt.Println("Write Temp file error:", scriptFile, err)
		return "", err
	}
	return RunOSCommandWithEnv(env, workingDir, "/bin/bash", scriptFile)
}

//InsenstiveContains is a function to check a contains b, and case insensitive
func InsenstiveContains(a, b string) bool {
	return strings.Contains(strings.ToLower(a), strings.ToLower(b))
}

//RunShellCmd is a function to run a shell command
func RunShellCmd(script string) (string, error) {
	return RunShellCmdWithEnv(nil, defTmpDir, script)
}

//RunShellEnvCmd is a function to run a shell command with environament variable
func RunShellEnvCmd(env []string, script string) (string, error) {
	return RunShellCmdWithEnv(env, defTmpDir, script)
}

//RunOSCommandWithArgs is a function to run a shell command in work dir
func RunOSCommandWithArgs(command string, arguments ...string) (string, error) {
	return RunOSCommandWithEnv(nil, "", command, arguments...)
}

//RunOSCommandWithEnv is a basic function to run a shell command
// If Env is not set, the process inherits environment of the calling process.
func RunOSCommandWithEnv(env []string, workingDir string, command string, arguments ...string) (string, error) {
	cmd := exec.Command(command, arguments...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdin = strings.NewReader("")
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	err := cmd.Run()
	return outb.String() + errb.String(), err
}

//PrintResultError  is a func to print terminal result and err info
func PrintResultError(result string, err error) {
	if err != nil {
		fmt.Println("result:\n", result, "err:\n", err)
	}
}

//Createkubeconfig is a func to get clusterdeployment kubeconfig and redirect to a file and return file path
func Createkubeconfig(clustername, namespace, workingDir string) (string, string, error) {
	tmpDir, err := ioutil.TempDir(workingDir, "hive")
	if err != nil {
		fmt.Println("Create Tmp Dir error:", err)
		return "", "", err
	}

	// clean up
	if workingDir == defTmpDir {
		// run in the tmpDir
		workingDir = tmpDir
	}
	cmd := `oc get cd %s -n %s -o jsonpath='{.spec.clusterMetadata.adminKubeconfigSecretRef.name}'`
	KubeconfigSecretRef, _ := RunShellCmd(fmt.Sprintf(cmd, clustername, namespace))
	cmd = `oc get secret %s -n %s -o json | jq -r '.data.kubeconfig' | base64 -d > %s/%s.kubeconfig`
	result, err := RunShellCmd(fmt.Sprintf(cmd, KubeconfigSecretRef, namespace, workingDir, clustername))
	PrintResultError(result, err)
	kubeconfigPath := fmt.Sprintf("%s/%s.kubeconfig", workingDir, clustername)
	return kubeconfigPath, workingDir, err
}
