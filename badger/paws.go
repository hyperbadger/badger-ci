package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	nomad "github.com/hashicorp/nomad/api"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

type Paws struct {
	Default    Settings     `hcl:"settings,block"`
	Stage      []Stages     `hcl:"stage,block"`
	Deployment []Deployment `hcl:"deployment,block"`
}

type Settings struct {
	LocalWeb         string        `hcl:"localweb,optional"`
	LocalPath        string        `hcl:"localpath,optional"`
	LocalInterface   string        `hcl:"localinterface,optional"`
	GitLabPath       string        `hcl:"gitlabpath,optional"`
	GitHubPath       string        `hcl:"githubpath,optional"`
	PathTo           string        `hcl:"pathto"`
	DefaultContainer string        `hcl:"default_container"`
	Environment      []Environment `hcl:"environment,block"`
}

type Environment struct {
	Name       string `hcl:"name,label"`
	Priority   string `hcl:"priority"`
	Region     string `hcl:"region"`
	Datacenter string `hcl:"datacenter"`
}

type Stages struct {
	Group    string  `hcl:"group,label"`
	SubGroup string  `hcl:"subgroup,label"`
	Include  string  `hcl:"include,optional"`
	Steps    []Steps `hcl:"step,block"`
}

type Steps struct {
	Name         string   `hcl:"name,label"`
	Driver       Driver   `hcl:"driver,block"`
	Command      []string `hcl:"command"`
	Environments []string `hcl:"environments"`
	Deployment   string   `hcl:"deployment,optional"`
	PathTo       string   `hcl:"pathto,optional"`
	WorkDir      string   `hcl:"workdir,optional"`
}

type StepsCollection struct {
	Steps []Steps `hcl:"step,block"`
}

type Driver struct {
	Name      string `hcl:"name,label"`
	Shell     string `hcl:"shell,optional"`
	Container string `hcl:"container,optional"`
}

type Deployment struct {
	DeploymentName string `hcl:"deployment,label"`
	Pack           string `hcl:"pack"`
}

// Main Entrypoint into Paws
func main() {
	args := os.Args[1:]

	var pawfile = args[0]
	var action = args[1]
	var section = args[2]

	switch action {
	case "run":
		pawsrunprocess(pawfile, action, section, false)
	case "valid":
		JSON, err := json.MarshalIndent(pawsconfig(pawfile), "", "  ")
		if err != nil {
			log.Fatalf("Failed to parse configuration: %s", pawfile)
		}
		log.Printf(string(JSON))
	default:
		log.Printf("Unknown mode: %s", action)
	}
}

// Parses Configuration File, pulls in includes and merges
func pawsconfig(pawfile string) Paws {
	var PawsTemp Paws
	err := hclsimple.DecodeFile(pawfile, nil, &PawsTemp)
	if err != nil {
		log.Fatalf("Failed to load configuration: %s", err)
	}

	for i, includes := range PawsTemp.Stage {
		if includes.Include != "" {
			var SC StepsCollection
			err := hclsimple.DecodeFile(includes.Include, nil, &SC)
			if err != nil {
				log.Fatalf("Failed to load configuration: %s", err)
			}
			//log.Printf("Include found %#v", SC.Steps)
			for _, step := range SC.Steps {
				PawsTemp.Stage[i].Steps = append(PawsTemp.Stage[i].Steps, step)
			}
		}
	}

	return PawsTemp
}

// Get Priority from any defined environments
func envpriority(Paws Paws, environ string) int {
	var pri int
	for _, env := range Paws.Default.Environment {
		if env.Name == environ {
			pri, _ = strconv.Atoi(env.Priority)
		}
	}
	return pri
}

// Get Region from any defined environments
func envregion(Paws Paws, environ string) string {
	var region string
	for _, env := range Paws.Default.Environment {
		if env.Name == environ {
			region = env.Region
		}
	}
	return region
}

// Get Datacenter from any defined environments
func envdc(Paws Paws, environ string) string {
	var dc string
	for _, env := range Paws.Default.Environment {
		if env.Name == environ {
			dc = env.Datacenter
		}
	}
	return dc
}

// Adding a task to the nomad job, if it's local it does something slightly different
func addPawsTask(Paws Paws, step Steps, ntg *nomad.TaskGroup, localonly bool) {
	dTask := nomad.NewTask(step.Name, step.Driver.Name)

	if localonly {

		localweb := Paws.Default.LocalWeb
		localip := GetInternalIP(Paws.Default.LocalInterface)

		source := fmt.Sprintf("%s/artifact.zip", strings.Replace(localweb, "{IP}", localip, 1))
		var destination string = Paws.Default.PathTo
		if step.PathTo != "" {
			destination = step.PathTo
		}
		log.Printf("source: %v", source)
		dTask.Artifacts = []*nomad.TaskArtifact{
			&nomad.TaskArtifact{
				GetterSource: &source,
				RelativeDest: &destination,
			},
		}
		sourcedata := strings.Join(step.Command, "\n")
		destpath := "local/run.sh"
		dTask.Templates = []*nomad.Template{
			&nomad.Template{
				EmbeddedTmpl: &sourcedata,
				DestPath:     &destpath,
			},
		}
	}

	dTaskCfg := make(map[string]interface{})
	if step.Driver.Name == "docker" {
		dTaskCfg["image"] = step.Driver.Container
		dTaskCfg["entrypoint"] = []string{"/bin/sh", "/local/run.sh"}
		dTaskCfg["work_dir"] = step.WorkDir

	}
	if step.Driver.Name == "raw_exec" {
		dTaskCfg["command"] = step.Driver.Shell
		dTaskCfg["args"] = step.Command
	}

	dTask.Config = dTaskCfg
	ntg.AddTask(dTask)

}

// The Run action function, basically the main business logic of Paws
func pawsrunprocess(pawfile string, action string, section string, dryrun bool) {

	var Paws Paws = pawsconfig(pawfile)

	log.Printf("Configuration is %#v", Paws)
	localonly := false
	log.Printf("Section is %s", section)

	var pri int
	var region string
	var datacenter string
	pri = envpriority(Paws, "remote")
	region = envregion(Paws, "remote")
	datacenter = envdc(Paws, "remote")

	if Paws.Default.LocalPath != "" {
		localZip(Paws)
		pri = envpriority(Paws, "local")
		region = envregion(Paws, "local")
		datacenter = envdc(Paws, "local")
		localonly = true
	}

	id := uuid.New()
	idn := fmt.Sprintf("%s-badger-paws", id.String())

	nbj := nomad.NewBatchJob(idn, idn, region, pri)
	nbj.AddDatacenter(datacenter)
	sctp := strings.Split(section, ".")
	var groupselect string = sctp[0]
	var subgroupselect string = "NONE"
	var minnumgroup int = 1
	if len(sctp) > minnumgroup {
		subgroupselect = sctp[1]

	}
	for _, stage := range Paws.Stage {
		if groupselect == stage.Group {
			if (subgroupselect == "NONE") || (subgroupselect == stage.SubGroup) {
				groupname := fmt.Sprintf("%s.%s", stage.Group, stage.SubGroup)
				ntg := nomad.NewTaskGroup(groupname, 1)
				var attempts int = 0
				ntg.ReschedulePolicy = &nomad.ReschedulePolicy{
					Attempts: &attempts,
				}
				ntg.RestartPolicy = &nomad.RestartPolicy{
					Attempts: &attempts,
				}
				nbj.AddTaskGroup(ntg)

				for _, step := range stage.Steps {
					addPawsTask(Paws, step, ntg, localonly)
				}
			}
		}

	}

	if !dryrun {
		nClient, err := nomad.NewClient(&nomad.Config{
			Address: os.Getenv("NOMAD_ADDR"),
		})
		if err != nil {
			log.Fatalf("error creating client: %v", err)
		}

		JobsAPI := nClient.Jobs()
		_, _, err = JobsAPI.RegisterOpts(
			nbj,
			&nomad.RegisterOptions{},
			&nomad.WriteOptions{},
		)
		if err != nil {
			log.Fatalf("error with job submission: %v", err)
		}
	} else {
		log.Printf("Running job with %#v", nbj)
	}
}

// A copy pasta function to return an IP from an interface name
func GetInternalIP(ifname string) string {
	itf, _ := net.InterfaceByName(ifname) //here your interface
	item, _ := itf.Addrs()
	var ip net.IP
	for _, addr := range item {
		switch v := addr.(type) {
		case *net.IPNet:
			if !v.IP.IsLoopback() {
				if v.IP.To4() != nil { //Verify if IP is IPV4
					ip = v.IP
				}
			}
		}
	}
	if ip != nil {
		return ip.String()
	} else {
		return ""
	}
}

// A helper function to zip the contents of the folder paws is running in
func localZip(Paws Paws) {
	localzip := fmt.Sprintf("%s/artifact.zip", Paws.Default.LocalPath)
	currentdir, err := os.Getwd()
	if err != nil {
		log.Printf("Error getting current directory: %v", err)
	}
	localcode := fmt.Sprintf("%s/", currentdir)
	log.Printf("Zip local folder: %s", localcode)
	zipit(localcode, localzip, true)
}

// A copy pasta function to zip a folder
func zipit(source, target string, needBaseDir bool) error {
	zipfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	info, err := os.Stat(source)
	if err != nil {
		return err
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		if baseDir != "" {
			if needBaseDir {
				header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
			} else {
				path := strings.TrimPrefix(path, source)
				if len(path) > 0 && (path[0] == '/' || path[0] == '\\') {
					path = path[1:]
				}
				if len(path) == 0 {
					return nil
				}
				header.Name = path
			}
		}

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})

	return err
}

// https://play.golang.org/p/Qg_uv_inCek
// contains checks if a string is present in a slice
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
