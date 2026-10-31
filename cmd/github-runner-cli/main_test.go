package main

import(
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func testAssets(t *testing.T)string{
 t.Helper()
 root:=t.TempDir()
 for _,path:=range []string{"runner/docker/Dockerfile","runner/04-run.sh","doctor-linux.sh"}{
  full:=filepath.Join(root,filepath.FromSlash(path))
  if e:=os.MkdirAll(filepath.Dir(full),0755);e!=nil{t.Fatal(e)}
  if e:=os.WriteFile(full,[]byte("fixture"),0644);e!=nil{t.Fatal(e)}
 }
 t.Setenv("GITHUB_RUNNER_CLI_ENV",root)
 return root
}
func TestHelpAndVersion(t *testing.T){
 for _,args:=range [][]string{nil,{"--help"},{"version"}}{
  if e:=run(args,"/tmp/github-runner-cli",t.TempDir());e!=nil{t.Fatal(e)}
 }
}
func TestAssetsError(t *testing.T){
 t.Setenv("GITHUB_RUNNER_CLI_ENV",filepath.Join(t.TempDir(),"missing"))
 if _,e:=assetsDir("/tmp/cli",t.TempDir());e==nil{t.Fatal("missing assets accepted")}
}
func TestBadArgumentsDoNotModifyRunnerState(t *testing.T){
 root:=testAssets(t)
 for _,args:=range [][]string{
  {"install"},{"install","r14r/BookBuilder","--bad"},
  {"install","r14r/BookBuilder","--name"},
  {"start"},{"stop","r14r/BookBuilder","extra"},
  {"runner"},{"runner","unknown","r14r/BookBuilder"},
  {"runner","register","invalid"},
  {"host"},{"host","unknown"},
 }{
  if e:=run(args,filepath.Join(root,"cli"),root);e==nil{t.Errorf("accepted invalid %v",args)}
 }
}
func TestRepositoryProfiles(t *testing.T){
 t.Setenv("RUNNER_PROFILE","")
 t.Setenv("RUNNER_IMAGE","")
 a,e:=newDockerRunner("r14r/BookBuilder",t.TempDir());if e!=nil{t.Fatal(e)}
 if a.Profile!="bookbuilder"||a.Image!="github-actions-runner-local:2.338.1-bookbuilder" {t.Fatalf("%+v",a)}
 b,e:=newDockerRunner("r14r/github-runner-test",t.TempDir());if e!=nil{t.Fatal(e)}
 if b.Profile!="runner-base"||b.Container!="github-runner-r14r-github-runner-test"{t.Fatalf("%+v",b)}
 if a.Volume==b.Volume{t.Fatal("different repositories must not share registration volumes")}
}
func TestExplicitProfileAndImage(t *testing.T){
 t.Setenv("RUNNER_PROFILE","bookbuilder")
 t.Setenv("RUNNER_IMAGE","test:image")
 r,e:=newDockerRunner("other/project",t.TempDir());if e!=nil{t.Fatal(e)}
 if r.Profile!="bookbuilder"||r.Image!="test:image"{t.Fatal(r)}
}
func TestProfileValidation(t *testing.T){
 t.Setenv("RUNNER_PROFILE","unknown")
 if _,e:=newDockerRunner("r14r/BookBuilder",t.TempDir());e==nil{t.Fatal("invalid profile accepted")}
}
func TestRepoSlug(t *testing.T){
 slug,e:=repoSlug("https://github.com/r14r/BookBuilder.git")
 if e!=nil||slug!="r14r-bookbuilder"{t.Fatalf("%q %v",slug,e)}
 for _,name:=range []string{"", "bad", "../../etc/passwd", "x/y/z"}{
  if _,e:=repoSlug(name);e==nil{t.Errorf("accepted %q",name)}
 }
}
func TestRegisterFlagParsing(t *testing.T){
 allow,name,e:=parseRegisterFlags([]string{"--allow-public","--name","runner-1"})
 if e!=nil||!allow||name!="runner-1"{t.Fatalf("%v %q %v",allow,name,e)}
 for _,args:=range [][]string{{"--name"},{"--name","--allow-public"},{"--bogus"}}{
  if _,_,e:=parseRegisterFlags(args);e==nil{t.Errorf("accepted %v",args)}
 }
}
func TestSetupOptions(t *testing.T){
 o,e:=parseSetup([]string{"r14r/BookBuilder","--workflow",".github/workflows/ci.yml","--job","python","--allow-public"})
 if e!=nil||o.repo!="r14r/BookBuilder"||len(o.workflows)!=1||o.jobs[0]!="python"||!o.allowPublic{t.Fatalf("%+v: %v",o,e)}
 for _,args:=range [][]string{nil,{"bad"},{"r14r/BookBuilder","--workflow"},{"r14r/BookBuilder","--unknown"}}{
  if _,e:=parseSetup(args);e==nil{t.Errorf("accepted %v",args)}
 }
}
func TestGoOnlyJustfile(t *testing.T){
 raw,e:=os.ReadFile(filepath.Join("..","..","justfile"));if e!=nil{t.Fatal(e)}
 just:=string(raw)
 for _,recipe:=range []string{"\nbootstrap:","\nci:","\nbuild:","\ninstall:"}{
  if !strings.Contains(just,recipe){t.Errorf("recipe %q missing",recipe)}
 }
 for _,forbidden:=range []string{"pytest","python3","patch_workflow.py"}{
  if strings.Contains(just,forbidden){t.Errorf("Python dependency remains: %s",forbidden)}
 }
}
