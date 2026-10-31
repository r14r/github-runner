package main

import (
 "os"
 "path/filepath"
 "testing"
)

func TestVersionAndHelp(t *testing.T) {
 cwd:=t.TempDir()
 for _,args:=range [][]string{{"version"},{"--help"},nil} {
  if err:=run(args,filepath.Join(cwd,"bin"),cwd);err!=nil {t.Fatalf("%v: %v",args,err)}
 }
}
func TestMissingAssets(t *testing.T) {
 t.Setenv("GITHUB_RUNNER_CLI_ENV","")
 tmp:=t.TempDir()
 if _,err:=assetsDir(filepath.Join(tmp,"bin"),tmp);err==nil {t.Fatal("missing runtime assets accepted")}
}
func TestInvalidCommands(t *testing.T) {
 t.Setenv("GITHUB_RUNNER_CLI_ENV","")
 cwd,err:=os.Getwd();if err!=nil {t.Fatal(err)}
 root:=filepath.Join(cwd,"..","..")
 for _,args:=range [][]string{{"runner"},{"runner","unsupported"},{"runner","download"},{"host"},{"host","unknown"}} {
  if err:=run(args,filepath.Join(root,"dist","github-runner-cli"),root);err==nil {t.Fatalf("accepted invalid args %v",args)}
 }
}
func TestAllEntrypointsExist(t *testing.T) {
 root:=filepath.Join("..","..")
 for _,file:=range runnerActions {
  if _,err:=os.Stat(filepath.Join(root,"runner",file));err!=nil {t.Fatalf("missing %s: %v",file,err)}
 }
 for _,file:=range hostActions {
  if _,err:=os.Stat(filepath.Join(root,file));err!=nil {t.Fatalf("missing %s: %v",file,err)}
 }
}
