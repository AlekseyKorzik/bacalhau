package ignite

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/filecoin-project/bacalhau/internal/system"
	"github.com/filecoin-project/bacalhau/internal/types"
)

const IGNITE_IMAGE string = "binocarlos/bacalhau-ignite-image:v1"

type Vm struct {
	Id   string
	Name string
	Job  *types.Job
}

func NewVm(job *types.Job) (*Vm, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s%s", job.Id, id.String())
	vm := &Vm{
		Id:   id.String(),
		Name: name,
		Job:  job,
	}
	return vm, nil
}

// start the vm so we can exec to prepare and run the job
func (vm *Vm) Start() error {
	return system.RunCommand("sudo", []string{
		"ignite",
		"run",
		IGNITE_IMAGE,
		"--name",
		vm.Name,
		"--cpus",
		fmt.Sprintf("%d", vm.Job.Cpu),
		"--memory",
		fmt.Sprintf("%dGB", vm.Job.Memory),
		"--size",
		fmt.Sprintf("%dGB", vm.Job.Disk),
		"--ssh",
	})
}

func (vm *Vm) Stop() error {
	return system.RunCommand("sudo", []string{
		"ignite",
		"rm",
		"-f",
		vm.Name,
	})
}

// create a script from the job commands
// these means we can run all commands as a single process
// that can be invoked by psrecord
// to do this - we need the commands inside the vm as a "job.sh" file
// (so we can "bash job.sh" as the command)
// let's write our "job.sh" and copy it onto the vm
func (vm *Vm) PrepareJob() error {
	tmpFile, err := ioutil.TempFile("", "bacalhau-ignite-job.*.sh")
	if err != nil {
		return err
	}
	defer tmpFile.Close()
	defer os.Remove(tmpFile.Name())
	// put sleep here because otherwise psrecord does not have enough time to capture metrics
	script := fmt.Sprintf("sleep 2\n%s\nsleep 2\n", strings.Join(vm.Job.Commands[:], "\n"))
	_, err = tmpFile.WriteString(script)
	if err != nil {
		return err
	}
	err = system.RunCommand("sudo", []string{
		"ignite",
		"cp",
		tmpFile.Name(),
		fmt.Sprintf("%s:/job.sh", vm.Name),
	})
	if err != nil {
		return err
	}
	err = system.RunCommand("sudo", []string{
		"ignite",
		"exec",
		vm.Name,
		"ipfs init",
	})
	if err != nil {
		return err
	}
	go func() {
		err := system.RunCommand("sudo", []string{
			"ignite",
			"exec",
			vm.Name,
			"ipfs daemon --mount",
		})
		if err != nil {
			log.Printf("Starting ipfs daemon --mount inside the vm failed with: %s", err)
		}

		// TODO: handle closing this when the job has finished
	}()

	// sleep here to give the "ipfs daemon --mount" command time to start
	time.Sleep(5 * time.Second)

	return nil
}

// TODO: mount input data files
// TODO: mount output data files
// psrecord invoke the job that we have prepared at /job.sh
// copy the psrecord metrics out of the vm
// TODO: bunlde the results data and metrics
func (vm *Vm) RunJob(resultsFolder string) error {

	err := vm.PrepareJob()

	if err != nil {
		return err
	}

	//nolint
	err, stdout, stderr := system.RunTeeCommand("sudo", []string{
		"ignite",
		"exec",
		vm.Name,
		fmt.Sprintf("psrecord 'bash /job.sh' --log /tmp/metrics.log --plot /tmp/metrics.png --include-children"),
	})

	// write the command stdout & stderr to the results dir
	fmt.Printf("writing stdout to %s/stdout.log\n", resultsFolder)
	err = os.WriteFile(fmt.Sprintf("%s/stdout.log", resultsFolder), stdout.Bytes(), 0644)
	if err != nil {
		return err
	}

	fmt.Printf("writing stderr to %s/stderr.log\n", resultsFolder)
	err = os.WriteFile(fmt.Sprintf("%s/stderr.log", resultsFolder), stderr.Bytes(), 0644)
	if err != nil {
		return err
	}

	// copy the psrecord metrics out of the vm
	filesToCopy := []string{
		"metrics.log",
		"metrics.png",
	}

	for _, file := range filesToCopy {
		fmt.Printf("writing %s to %s/%s\n", file, resultsFolder, file)
		err = system.RunCommand("sudo", []string{
			"ignite",
			"cp",
			fmt.Sprintf("%s:/tmp/%s", vm.Name, file),
			fmt.Sprintf("%s/%s", resultsFolder, file),
		})
	}

	return err
}
