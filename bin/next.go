package main

import (
 "bufio"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "strings"
)

type PR struct {
 Number int `json:"number"`
 Title string `json:"title"`
 URL string `json:"url"`
 Head string `json:"headRefName"`
 Base string `json:"baseRefName"`
 Draft bool `json:"isDraft"`
 Mergeable string `json:"mergeable"`
 MergeState string `json:"mergeStateStatus"`
 Review string `json:"reviewDecision"`
 Checks []struct { Conclusion string `json:"conclusion"`; State string `json:"state"`; Status string `json:"status"` } `json:"statusCheckRollup"`
}
func output(name string, args ...string) (string,error) {
 c:=exec.Command(name,args...); b,e:=c.CombinedOutput(); if e!=nil {return "",fmt.Errorf("%s %s: %w: %s",name,strings.Join(args," "),e,strings.TrimSpace(string(b)))}; return strings.TrimSpace(string(b)),nil
}
func run(name string,args ...string) error {
 fmt.Printf("  $ %s %s\n",name,strings.Join(args," "))
 c:=exec.Command(name,args...);c.Stdin=os.Stdin;c.Stdout=os.Stdout;c.Stderr=os.Stderr;return c.Run()
}
func checks(p PR) string {
 if len(p.Checks)==0{return "NO CHECKS"}
 pending:=false
 for _,c:=range p.Checks {
  state:=strings.ToUpper(c.Conclusion)
  if state=="" {state=strings.ToUpper(c.State)}
  if state=="" {state=strings.ToUpper(c.Status)}
  switch state {
  case "FAILURE","ERROR","TIMED_OUT","ACTION_REQUIRED","CANCELLED":return "FAIL"
  case "SUCCESS","NEUTRAL","SKIPPED": 
  default:pending=true
  }
 }
 if pending{return "PENDING"}
 return "PASS"
}
func ghJSON(target any,args ...string)error{v,e:=output("gh",args...);if e!=nil{return e};return json.Unmarshal([]byte(v),target)}
func main(){if err:=next();err!=nil{fmt.Fprintln(os.Stderr,"ERROR:",err);os.Exit(1)}}
func next()error {
 execute:=len(os.Args)>1&&os.Args[1]=="--run"
 if len(os.Args)>1 && (!execute||len(os.Args)>2){return fmt.Errorf("usage: just next [--run]")}
 branch,e:=output("git","branch","--show-current");if e!=nil{return e}
 status,e:=output("git","status","--porcelain");if e!=nil{return e}
 var repo struct {Name string `json:"nameWithOwner"`}
 if e=ghJSON(&repo,"repo","view","--json","nameWithOwner");e!=nil{return e}
 var prs []PR
 if e=ghJSON(&prs,"pr","list","--repo",repo.Name,"--state","open","--limit","100","--json","number,title,url,headRefName,baseRefName,isDraft,mergeable,mergeStateStatus,reviewDecision,statusCheckRollup");e!=nil{return e}
 fmt.Printf("CURRENT BRANCH: %s\n",branch)
 if status!=""{fmt.Println("WORKTREE: dirty")}else{fmt.Println("WORKTREE: clean")}
 fmt.Printf("REPOSITORY: %s\nOPEN PULL REQUESTS: %d\n",repo.Name,len(prs))
 var current *PR
 for i:=range prs {
  p:=&prs[i];if p.Head==branch{current=p}
  fmt.Printf("  #%d %s\n    %s -> %s | checks=%s, merge=%s\n    %s\n",p.Number,p.Title,p.Head,p.Base,checks(*p),p.MergeState,p.URL)
 }
 if len(prs)==0{fmt.Println("  No open pull requests.")}
 fmt.Println("\nNEXT STEPS — local validation")
 fmt.Println("  Note: just build includes bootstrap, format, vet, unit tests, and race tests.")
 if !execute {
  fmt.Println("  $ git diff --check\n  $ just build\n  Add --run to execute these checks.")
 } else {
  if e=run("git","diff","--check");e!=nil{return e}
  if e=run("just","build");e!=nil{return e}
 }
 fmt.Println("\nNEXT STEPS — pull requests")
 if len(prs)==0{fmt.Println("  No PR to accept.");return nil}
 for _,p:=range prs {
  fmt.Printf("  PR #%d (%s):\n",p.Number,checks(p))
  fmt.Printf("    $ gh pr checks %d --repo %s\n",p.Number,repo.Name)
  if p.Draft {fmt.Println("    Mark draft as ready after review.")}
  if p.Mergeable=="CONFLICTING" || p.MergeState=="DIRTY"{fmt.Println("    Resolve merge conflicts.")}
  if p.Head!=branch {fmt.Printf("    Review separately: gh pr checkout %d && just build\n",p.Number)}
  if execute {
   v,err:=output("gh","pr","checks",fmt.Sprint(p.Number),"--repo",repo.Name)
   if err!=nil {fmt.Printf("    GitHub checks unavailable or failing: %v\n",err)}else{fmt.Println(v)}
  }
 }
 if !execute||current==nil{return nil}
 var fresh PR
 if e=ghJSON(&fresh,"pr","view",fmt.Sprint(current.Number),"--repo",repo.Name,"--json","isDraft,mergeable,mergeStateStatus,statusCheckRollup,reviewDecision");e!=nil{return e}
 if status!=""||fresh.Draft||fresh.Mergeable!="MERGEABLE"||checks(fresh)!="PASS"||fresh.Review=="REVIEW_REQUIRED"||fresh.Review=="CHANGES_REQUESTED"{
  fmt.Printf("  PR #%d not eligible for merge (checks=%s, mergeable=%s).\n",current.Number,checks(fresh),fresh.Mergeable)
  return nil
 }
 fmt.Printf("Merge current PR #%d with --squash --delete-branch? [y/N]: ",current.Number)
 reader:=bufio.NewReader(os.Stdin);answer,e:=reader.ReadString('\n');if e!=nil{return e}
 answer=strings.ToLower(strings.TrimSpace(answer));if answer!="y"&&answer!="yes"{fmt.Println("Merge skipped.");return nil}
 return run("gh","pr","merge",fmt.Sprint(current.Number),"--repo",repo.Name,"--squash","--delete-branch")
}
