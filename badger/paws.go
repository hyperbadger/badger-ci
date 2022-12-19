package main

import (
	"archive/zip"
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
	LocalWeb         string   `hcl:"localweb,optional"`
	LocalPath        string   `hcl:"localpath,optional"`
	LocalInterface   string   `hcl:"localinterface,optional"`
	GitLabPath       string   `hcl:"gitlabpath,optional"`
	GitHubPath       string   `hcl:"githubpath,optional"`
	Priority         string   `hcl:"priority"`
	Region           string   `hcl:"region"`
	Datacenter       string   `hcl:"datacenter"`
	PathTo           string   `hcl:"pathto"`
	DefaultContainer string   `hcl:"default_container"`
	Environments     []string `hcl:"environments"`
}

type Stages struct {
	Group    string  `hcl:"group,label"`
	SubGroup string  `hcl:"subgroup,label"`
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

type Driver struct {
	Name      string `hcl:"name,label"`
	Shell     string `hcl:"shell,optional"`
	Container string `hcl:"container,optional"`
}

type Deployment struct {
	DeploymentName string `hcl:"deployment,label"`
	Pack           string `hcl:"pack"`
}

func main() {
	args := os.Args[1:]

	var pawfile = args[0]
	var action = args[1]
	var section = args[2]

	var Paws Paws
	err := hclsimple.DecodeFile(pawfile, nil, &Paws)
	if err != nil {
		log.Fatalf("Failed to load configuration: %s", err)
	}
	log.Printf("Configuration is %#v", Paws)
	if action == "run" {
		localonly := false
		log.Printf("Section is %s", section)
		if Paws.Default.LocalPath != "" {
			localZip(Paws)
			localonly = true
		}
		id := uuid.New()
		idn := fmt.Sprintf("%s-badger-paws", id.String())
		pri, _ := strconv.Atoi(Paws.Default.Priority)
		nbj := nomad.NewBatchJob(idn, idn, Paws.Default.Region, pri)
		nbj.AddDatacenter(Paws.Default.Datacenter)
		sctp := strings.Split(section, ".")
		for _, stage := range Paws.Stage {
			if contains(sctp, stage.Group) || contains(sctp, stage.SubGroup) {
				groupname := fmt.Sprintf("%s.%s", stage.Group, stage.SubGroup)
				ntg := nomad.NewTaskGroup(groupname, 1)
				nbj.AddTaskGroup(ntg)
				for _, step := range stage.Steps {
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
			}

		}
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
		log.Printf("Unknown run mode")
	}
}

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
