package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "sort"
    "strings"
)

const runnerContainerPrefix = "github-runner-"
const runnerVolumeSuffix = "-state"

type runnerRecord struct {
    ID int64 `json:"id"`
    Name string `json:"name"`
    Status string `json:"status"`
    Busy bool `json:"busy"`
    Labels []struct{Name string `json:"name"`} `json:"labels"`
}

type localRunner struct {
    Slug string
    Repository string
    Container string
    Volume string
    State string
    Identity runnerIdentity
}

type runnerIdentity struct {
    AgentID int64 `json:"agentId"`
    AgentName string `json:"agentName"`
    GitHubURL string `json:"gitHubUrl"`
}

type uninstallOptions struct {
    Repo string
    All bool
    Yes bool
    DeleteData bool
}

func parseUninstall(repo string, args []string) (uninstallOptions, error) {
    o := uninstallOptions{Repo:repo}
    for _, a := range args {
        switch a {
        case "--all": o.All = true
        case "--yes": o.Yes = true
        case "--delete-data": o.DeleteData = true
        default: return o, fmt.Errorf("uninstall: unknown option %q",a)
        }
    }
    if o.Repo != "" && o.All { return o,errors.New("use either --repo or --all, not both") }
    if o.Repo == "" && !o.All { return o,errors.New("uninstall requires --repo OWNER/REPO or --all --yes") }
    if o.All && !o.Yes { return o,errors.New("uninstall --all requires explicit --yes; nothing changed") }
    return o,nil
}

func installedSlugs() ([]string,error) {
    // Only names from our own precise namespace; other containers/volumes
    // remain completely outside the inventory and removal scope.
    containers,err:=commandOutput("","docker","ps","-a","--format","{{.Names}}")
    if err!=nil{return nil,err}
    volumes,err:=commandOutput("","docker","volume","ls","--format","{{.Name}}")
    if err!=nil{return nil,err}
    uniq:=map[string]bool{}
    for _,name:=range strings.Split(containers,"\n") {
        if strings.HasPrefix(name,runnerContainerPrefix) {
            slug:=strings.TrimPrefix(name,runnerContainerPrefix)
            if slug!="" {uniq[slug]=true}
        }
    }
    for _,name:=range strings.Split(volumes,"\n") {
        if strings.HasPrefix(name,runnerContainerPrefix)&&strings.HasSuffix(name,runnerVolumeSuffix) {
            slug:=strings.TrimSuffix(strings.TrimPrefix(name,runnerContainerPrefix),runnerVolumeSuffix)
            if slug!=""{uniq[slug]=true}
        }
    }
    slugs:=make([]string,0,len(uniq))
    for slug:=range uniq {slugs=append(slugs,slug)}
    sort.Strings(slugs)
    return slugs,nil
}

func existingContainer(name string)bool {
    return exec.Command("docker","container","inspect",name).Run()==nil
}
func existingVolume(name string)bool {
    return exec.Command("docker","volume","inspect",name).Run()==nil
}
func localBySlug(slug string)(localRunner,error) {
    l:=localRunner{Slug:slug,Container:runnerContainerPrefix+slug,
        Volume:runnerContainerPrefix+slug+runnerVolumeSuffix,State:"not-installed"}
    if existingContainer(l.Container) {
        status,err:=commandOutput("","docker","container","inspect","--format","{{.State.Status}}",l.Container)
        if err!=nil{return l,err}
        l.State=status
    }
    return l,nil
}

// Identity comes from the persisted GitHub runner configuration; we never
// infer OWNER/REPO from a hyphenated slug or delete a remote runner by label.
func readRunnerIdentity(l localRunner)(runnerIdentity,error){
    if !existingVolume(l.Volume) {return runnerIdentity{},nil}
    images:=[]string{}
    if existingContainer(l.Container){
        candidate,err:=commandOutput("","docker","container","inspect","--format","{{.Config.Image}}",l.Container)
        if err!=nil{return runnerIdentity{},err}
        images=append(images,candidate)
    }
    images=append(images,"github-actions-runner-local:2.338.1-bookbuilder","github-actions-runner-local:2.338.1")
    if override:=os.Getenv("RUNNER_IMAGE");override!=""{images=append([]string{override},images...)}
    seen:=map[string]bool{}
    for _,img:=range images{
        if seen[img]||img==""{continue}
        seen[img]=true
        if exec.Command("docker","image","inspect",img).Run()!=nil{continue}
        // Read-only volume mount: reading identity cannot initialize or mutate
        // the runner, unlike calling its entrypoint.
        cmd:=exec.Command("docker","run","--rm","--platform",dockerPlatform,
            "--mount","type=volume,source="+l.Volume+",target=/runner,readonly",
            "--entrypoint","sh",img,"-c",
            "if [ -f /runner/.runner ]; then cat /runner/.runner; else printf '{}'; fi")
        body,err:=cmd.Output()
        if err!=nil{return runnerIdentity{},fmt.Errorf("unable to inspect registration volume %s: %w",l.Volume,err)}
        var id runnerIdentity
        if err:=json.Unmarshal(body,&id);err!=nil{return id,fmt.Errorf("invalid runner identity in %s: %w",l.Volume,err)}
        if id.GitHubURL!=""{
            repo:=gitHubRepository(id.GitHubURL)
            if _,err:=repoSlug(repo);err==nil{return id,nil}
            return id,fmt.Errorf("invalid GitHub repository URL in runner identity: %q",id.GitHubURL)
        }
        return id,nil
    }
    return runnerIdentity{},fmt.Errorf("cannot inspect %s: no local runner image available; volume preserved",l.Volume)
}
func inspectLocal(slug string)(localRunner,error){
    l,err:=localBySlug(slug);if err!=nil{return l,err}
    if existingVolume(l.Volume) {
        id,err:=readRunnerIdentity(l);if err!=nil{return l,err}
        l.Identity=id
        if id.GitHubURL!=""{l.Repository=gitHubRepository(id.GitHubURL)}
    }
    if existingContainer(l.Container) {
        label,err:=commandOutput("","docker","container","inspect","--format",
            `{{index .Config.Labels "com.github-runner-cli.repository"}}`,l.Container)
        if err!=nil{return l,err}
        if label!="<no value>"&&label!=""{
            if _,err:=repoSlug(label);err==nil{
                if l.Repository!=""&&!strings.EqualFold(l.Repository,label){
                    return l,fmt.Errorf("container repository label conflicts with persisted identity: %s vs %s",label,l.Repository)
                }
                l.Repository=label
            }
        }
    }
    return l,nil
}

func githubRunners(repo string)([]runnerRecord,error){
    if _,err:=repoSlug(repo);err!=nil{return nil,err}
    // GH --paginate + jq produces one JSON object per line, not separate
    // JSON documents per page. Errors are fatal during uninstall preflight.
    out,err:=commandOutput("","gh","api","--paginate",
        "repos/"+repo+"/actions/runners?per_page=100","--jq",".runners[] | @json")
    if err!=nil{return nil,fmt.Errorf("cannot inspect GitHub runner registrations for %s: %w",repo,err)}
    result:=[]runnerRecord{}
    for _,line:=range strings.Split(out,"\n"){
        line=strings.TrimSpace(line);if line==""{continue}
        var item runnerRecord
        if err:=json.Unmarshal([]byte(line),&item);err!=nil{return nil,fmt.Errorf("invalid GitHub runner response: %w",err)}
        result=append(result,item)
    }
    return result,nil
}
func matchedRegistration(id runnerIdentity, remote []runnerRecord)(*runnerRecord,error){
    if id.AgentName==""{return nil,nil}
    for i:=range remote {
        if remote[i].Name==id.AgentName{
            if id.AgentID!=0&&remote[i].ID!=id.AgentID {
                return nil,fmt.Errorf("runner ID mismatch for %s (local %d, remote %d)",id.AgentName,id.AgentID,remote[i].ID)
            }
            return &remote[i],nil
        }
    }
    return nil,nil
}

func listInstalled(repo string)error{
    if selectedBackend()!="docker"{return errors.New("list-installed currently supports Docker/Colima runners only")}
    slugs,err:=installedSlugs();if err!=nil{return err}
    if repo!=""{
        slug,err:=repoSlug(repo);if err!=nil{return err}
        slugs=[]string{}
        l,e:=localBySlug(slug)
        if e!=nil{return e}
        if existingContainer(l.Container)||existingVolume(l.Volume){slugs=append(slugs,slug)}
    }
    fmt.Println("LOCAL DOCKER RUNNERS (this Docker context)")
    fmt.Println("REPOSITORY/SLUG\tCONTAINER\tSTATUS\tVOLUME\tRUNNER NAME")
    for _,slug:=range slugs {
        l,err:=inspectLocal(slug)
        if err!=nil{
            fmt.Printf("%s\t%s\t%s\t%s\t(identity unavailable: %v)\n",slug,runnerContainerPrefix+slug,"unknown",runnerContainerPrefix+slug+runnerVolumeSuffix,err)
            continue
        }
        name:=l.Repository
        if name==""{name=slug+" (repository unknown)"}
        volume:="missing"
        if existingVolume(l.Volume){volume=l.Volume}
        fmt.Printf("%s\t%s\t%s\t%s\t%s\n",name,l.Container,l.State,volume,l.Identity.AgentName)
    }
    if len(slugs)==0{fmt.Println("(none)")}
    if repo!=""{
        remote,err:=githubRunners(repo);if err!=nil{return err}
        fmt.Printf("\nGITHUB RUNNER REGISTRATIONS (%s, may include other hosts)\n",repo)
        fmt.Println("ID\tNAME\tSTATUS\tBUSY")
        for _,x:=range remote{fmt.Printf("%d\t%s\t%s\t%t\n",x.ID,x.Name,x.Status,x.Busy)}
        if len(remote)==0{fmt.Println("(none)")}
    }
    return nil
}

type removePlan struct{
    Local localRunner
    Remote *runnerRecord
}

// Preflight all candidates first: if any remote registration is busy, missing
// credentials or identity cannot be read, do not modify any runner.
func planRemoval(repos []string, explicitRepo string)([]removePlan,error){
    plans:=make([]removePlan,0,len(repos))
    remoteCache:=map[string][]runnerRecord{}
    for _,slug:=range repos{
        l,err:=inspectLocal(slug);if err!=nil{return nil,err}
        if explicitRepo!="" {
            if l.Repository!="" && !strings.EqualFold(l.Repository,explicitRepo){
                return nil,fmt.Errorf("runner belongs to %s, not %s",l.Repository,explicitRepo)
            }
            l.Repository=explicitRepo
        }
        if l.Repository=="" {
            return nil,fmt.Errorf("cannot determine repository for %s; remove it individually with --repo, not --all",slug)
        }
        if _,err:=repoSlug(l.Repository);err!=nil{return nil,err}
        if _,ok:=remoteCache[l.Repository];!ok{
            records,err:=githubRunners(l.Repository);if err!=nil{return nil,err}
            remoteCache[l.Repository]=records
        }
        reg,err:=matchedRegistration(l.Identity,remoteCache[l.Repository])
        if err!=nil{return nil,err}
        if reg!=nil&&reg.Busy {
            return nil,fmt.Errorf("%s runner %s is busy; wait for job completion (nothing removed)",l.Repository,reg.Name)
        }
        plans=append(plans,removePlan{Local:l,Remote:reg})
    }
    return plans,nil
}

func executeRemoval(plans []removePlan,deleteData bool)error{
    for _,p:=range plans{
        l:=p.Local
        fmt.Printf("Removing runner %s (%s)\n",l.Repository,l.Slug)
        if existingContainer(l.Container){
            running,err:=commandOutput("","docker","container","inspect","--format","{{.State.Running}}",l.Container)
            if err!=nil{return err}
            if running=="true"{
                if err:=commandRun("","docker","stop",l.Container);err!=nil{return fmt.Errorf("could not stop %s: %w",l.Container,err)}
            }
        }
        if p.Remote!=nil{
            if err:=commandRun("","gh","api","-X","DELETE",
                fmt.Sprintf("repos/%s/actions/runners/%d",l.Repository,p.Remote.ID));err!=nil{
                return fmt.Errorf("GitHub unregister failed for %s: %w (local data retained)",p.Remote.Name,err)
            }
            fmt.Println("Unregistered on GitHub:",p.Remote.Name)
        }
        if existingContainer(l.Container){
            if err:=commandRun("","docker","rm",l.Container);err!=nil{return err}
        }
        if deleteData&&existingVolume(l.Volume){
            if err:=commandRun("","docker","volume","rm",l.Volume);err!=nil{return err}
        }else if existingVolume(l.Volume){
            fmt.Println("Registration/workspace volume retained:",l.Volume,"(remove with --delete-data)")
        }
    }
    return nil
}

func uninstallCommand(repo string,args []string)error{
    o,err:=parseUninstall(repo,args);if err!=nil{return err}
    if selectedBackend()!="docker"{return errors.New("uninstall currently supports Docker/Colima runners only; native host services are not modified")}
    slugs:=[]string{}
    if o.All{
        slugs,err=installedSlugs();if err!=nil{return err}
    }else{
        slug,err:=repoSlug(o.Repo);if err!=nil{return err}
        l,err:=localBySlug(slug);if err!=nil{return err}
        if !existingContainer(l.Container)&&!existingVolume(l.Volume){
            return fmt.Errorf("no locally installed runner found for %s",o.Repo)
        }
        slugs=[]string{slug}
    }
    if len(slugs)==0{fmt.Println("No locally installed runners found.");return nil}
    plans,err:=planRemoval(slugs)
    if err!=nil{
        // With --repo, a legacy registration lacking gitHubUrl can still be
        // selected explicitly, but never override a conflicting identity.
        if !o.All && strings.Contains(err.Error(),"cannot determine repository"){
            return fmt.Errorf("%w; cannot verify runner identity without a repository URL",err)
        }
        return err
    }
    if !o.All&&!strings.EqualFold(plans[0].Local.Repository,o.Repo){
        return fmt.Errorf("runner belongs to %s, not %s; refusing to uninstall",plans[0].Local.Repository,o.Repo)
    }
    return executeRemoval(plans,o.DeleteData)
}

// self-uninstall intentionally does NOT unregister or stop any runners.
// The binary must be the actual installed executable next to its own assets.
// Refuse an unsafe checkout, temp go run binary, symlink, or overridden env.
func selfUninstall(executable string,args []string)error{
    if len(args)!=1||args[0]!="--yes"{return errors.New("self-uninstall requires --yes; does not remove registered runners")}
    if os.Getenv("GITHUB_RUNNER_CLI_ENV")!=""{
        return errors.New("GITHUB_RUNNER_CLI_ENV is set; unset it before self-uninstall")
    }
    binary,err:=filepath.Abs(executable);if err!=nil{return err}
    if filepath.Base(binary)!="github-runner-cli"{return errors.New("self-uninstall must run the installed github-runner-cli binary (not go run)")}
    stat,err:=os.Lstat(binary);if err!=nil{return err}
    if !stat.Mode().IsRegular(){return errors.New("refusing symlink or non-regular CLI binary")}
    assets:=filepath.Join(filepath.Dir(binary),"env","github-runner-cli")
    info,err:=os.Lstat(assets);if err!=nil{return fmt.Errorf("installed runtime assets missing: %w",err)}
    if !info.IsDir()||info.Mode()&os.ModeSymlink!=0{return errors.New("unsafe runtime assets path")}
    if _,err:=os.Stat(filepath.Join(assets,"runner","docker","Dockerfile"));err!=nil{
        return fmt.Errorf("runtime directory is not a CLI installation: %w",err)
    }
    if err:=os.RemoveAll(assets);err!=nil{return fmt.Errorf("unable to remove %s: %w",assets,err)}
    if err:=os.Remove(binary);err!=nil{return fmt.Errorf("assets removed, but CLI binary could not be removed: %w",err)}
    fmt.Printf("Removed %s and %s\n",binary,assets)
    fmt.Println("Existing GitHub registrations, Docker containers, images and volumes were not touched.")
    return nil
}
