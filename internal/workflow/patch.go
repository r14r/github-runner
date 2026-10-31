package workflow

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "regexp"
 "sort"
 "strings"
)

var jobLine = regexp.MustCompile(`^  ([A-Za-z0-9_.-]+):\s*(?:#.*)?$`)
var runsOn = regexp.MustCompile(`^    runs-on:\s*(.*?)\s*$`)
var labelValid = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var jobsHeader = regexp.MustCompile(`^jobs:\s*(?:#.*)?$`)

type Options struct {
 Label string
 OS string
 AllowCrossOS bool
 Jobs []string
}
type plan struct {
 path string
 before []byte
 after []byte
 found map[string]bool
 changed int
}
func patchOne(path string, opts Options, selected map[string]bool) (plan,error) {
 p:=plan{path:path,found:make(map[string]bool)}
 clean:=filepath.ToSlash(filepath.Clean(path))
 if !(strings.HasPrefix(clean,".github/workflows/")||strings.Contains(clean,"/.github/workflows/")) ||
  !(strings.HasSuffix(clean,".yml")||strings.HasSuffix(clean,".yaml")) {
  return p,fmt.Errorf("not a GitHub Actions workflow: %s",path)
 }
 stat,err:=os.Lstat(path);if err!=nil{return p,err}
 if !stat.Mode().IsRegular(){return p,fmt.Errorf("unsafe workflow file: %s",path)}
 p.before,err=os.ReadFile(path);if err!=nil{return p,err}
 // SplitAfter retains original LF/CRLF style, including no trailing newline.
 lines:=strings.SplitAfter(string(p.before),"\n")
 inside:=false
 current:=""
 target:=fmt.Sprintf("[self-hosted, %s, X64, %s]",opts.OS,opts.Label)
 for i,line:=range lines {
  raw:=strings.TrimRight(line,"\r\n")
  if jobsHeader.MatchString(raw){inside=true;current="";continue}
  if !inside || strings.TrimSpace(raw)==""||strings.HasPrefix(strings.TrimLeft(raw," \t"),"#"){continue}
  if !strings.HasPrefix(raw," "){inside=false;current="";continue}
  if m:=jobLine.FindStringSubmatch(raw);m!=nil{current=m[1];continue}
  if current==""||(len(selected)>0&&!selected[current]){continue}
  if !strings.HasPrefix(raw,"    ")||strings.HasPrefix(raw,"     "){continue}
  m:=runsOn.FindStringSubmatch(raw);if m==nil{continue}
  p.found[current]=true
  orig:=strings.TrimSpace(m[1]);if orig==target{continue}
  if orig==""||strings.Contains(orig,"${{")||strings.HasPrefix(orig,"!")||strings.HasPrefix(orig,"*")||strings.HasPrefix(orig,"&") {
    return p,fmt.Errorf("%s:%d: dynamic/multiline runs-on; edit manually",path,i+1)
  }
  if strings.Contains(orig,"#")||strings.HasPrefix(orig,"[")&&!strings.HasSuffix(orig,"]") {
    return p,fmt.Errorf("%s:%d: multiline/inline comment runs-on; edit manually",path,i+1)
  }
  lower:=strings.ToLower(orig)
  prev:=""
  switch {
   case strings.Contains(lower,"windows")||strings.Contains(lower,"win-"):prev="Windows"
   case strings.Contains(lower,"macos")||strings.Contains(lower,"osx"):prev="macOS"
   case strings.Contains(lower,"ubuntu")||strings.Contains(lower,"linux"):prev="Linux"
  }
  if prev!=""&&prev!=opts.OS&&!opts.AllowCrossOS {
    return p,fmt.Errorf("%s:%d: %s -> %s requires --allow-cross-os",path,i+1,prev,opts.OS)
  }
  ending:="";if strings.HasSuffix(line,"\r\n"){ending="\r\n"}else if strings.HasSuffix(line,"\n"){ending="\n"}
  lines[i]="    runs-on: "+target+ending
  p.changed++
 }
 p.after=[]byte(strings.Join(lines,""))
 return p,nil
}

// Patch plans and validates all files before modifying any of them. On write
// failure, already-modified files are restored from the validated snapshots.
func Patch(paths []string, opts Options)(int,error){
 if !labelValid.MatchString(opts.Label)||(opts.OS!="Linux"&&opts.OS!="macOS"){
  return 0,errors.New("invalid runner OS/label")
 }
 if len(paths)==0{return 0,errors.New("no workflow files supplied")}
 selected:=make(map[string]bool)
 for _,job:=range opts.Jobs{selected[job]=true}
 plans:=make([]plan,0,len(paths));found:=make(map[string]bool)
 count:=0
 for _,path:=range paths{
  p,err:=patchOne(path,opts,selected);if err!=nil{return 0,err}
  for name:=range p.found{found[name]=true}
  count+=p.changed;plans=append(plans,p)
 }
 for job:=range selected{
  if !found[job]{return 0,fmt.Errorf("job %q not found or not patchable",job)}
 }
 if len(found)==0{return 0,errors.New("no supported job-level runs-on found")}
 modified:=make([]plan,0,len(plans))
 for _,p:=range plans{
  if string(p.after)==string(p.before){continue}
  if err:=os.WriteFile(p.path,p.after,0644);err!=nil{
   for _,prev:=range modified{_ =os.WriteFile(prev.path,prev.before,0644)}
   return 0,err
  }
  modified=append(modified,p)
 }
 for _,p:=range plans{fmt.Printf("%s: %d job(s) changed\n",p.path,p.changed)}
 return count,nil
}
func AvailableJobs(path string)([]string,error){
 b,err:=os.ReadFile(path);if err!=nil{return nil,err}
 out:=[]string{};for _,line:=range strings.Split(string(b),"\n"){
  if m:=jobLine.FindStringSubmatch(strings.TrimRight(line,"\r"));m!=nil{out=append(out,m[1])}
 }
 sort.Strings(out);return out,nil
}
