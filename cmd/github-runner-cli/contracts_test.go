package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenericRunnerEntrypointIsValidShell(t *testing.T) {
    path:=filepath.Join("..","..","runner","docker","entrypoint.sh")
    out,err:=exec.Command("bash","-n",path).CombinedOutput()
    if err!=nil{t.Fatalf("invalid shell entrypoint: %s: %v",out,err)}
    for _,legacy:=range []string{"scripts/install-macos.sh","scripts/install-ubuntu.sh",
        "runner/01-download.sh","runner/02-configure.sh","runner/04-run.sh"} {
        if _,err:=os.Stat(filepath.Join("..","..",legacy));!os.IsNotExist(err){
            t.Errorf("obsolete host-specific asset still exists: %s",legacy)
        }
    }
}

func TestDockerEntrypointRetainsVolumeAndTokenOnStdin(t *testing.T) {
	body, e := os.ReadFile(filepath.Join("..", "..", "runner", "docker", "entrypoint.sh"))
	if e != nil {
		t.Fatal(e)
	}
	s := string(body)
	for _, want := range []string{"cp -a /opt/actions-runner/. /runner/", "read -r token", "--unattended", "gosu runner"} {
		if !strings.Contains(s, want) {
			t.Errorf("Docker entrypoint missing %s", want)
		}
	}
}
func TestGenericDockerImageUsesSafeAllowlistedToolchain(t *testing.T) {
    body,err:=os.ReadFile(filepath.Join("..","..","runner","docker","Dockerfile"))
    if err!=nil{t.Fatal(err)}
    source:=string(body)
    for _,want:=range []string{"FROM ubuntu:24.04 AS runner-base",
        "FROM runner-base AS toolchain","EXTRA_APT_PACKAGES",
        "INSTALL_JUST=0","INSTALL_TYPST=0","sha256sum -c -"}{
        if !strings.Contains(source,want){t.Errorf("generic image missing %s",want)}
    }
    for _,forbidden:=range []string{"AS bookbuilder","ppa:ondrej/php",
        "trusted=yes","AllowInsecureRepositories"}{
        if strings.Contains(source,forbidden){t.Errorf("unsafe or target-specific image reference: %s",forbidden)}
    }
}

func TestOnlyGoSourceForWorkflowPatcher(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, file := range []string{"runner/lib/patch_workflow.py", "runner/03-setup.sh", "tests/test_runner.py"} {
		if _, e := os.Stat(filepath.Join(root, file)); !os.IsNotExist(e) {
			t.Errorf("legacy Python workflow implementation still present: %s", file)
		}
	}
}
