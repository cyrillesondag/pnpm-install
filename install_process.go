package pnpminstall

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paketo-buildpacks/packit/v2/fs"
	"github.com/paketo-buildpacks/packit/v2/pexec"
	"github.com/paketo-buildpacks/packit/v2/scribe"
)

//go:generate faux --interface Summer --output fakes/summer.go
type Summer interface {
	Sum(paths ...string) (string, error)
}

//go:generate faux --interface Executable --output fakes/executable.go
type Executable interface {
	Execute(pexec.Execution) error
}

type PnpmInstallProcess struct {
	executable Executable
	summer     Summer
	logger     scribe.Emitter
}

func NewPnpmInstallProcess(executable Executable, summer Summer, logger scribe.Emitter) PnpmInstallProcess {
	return PnpmInstallProcess{
		executable: executable,
		summer:     summer,
		logger:     logger,
	}
}

func (ip PnpmInstallProcess) ShouldRun(workingDir string, metadata map[string]interface{}) (run bool, sha string, err error) {
	ip.logger.Subprocess("Process inputs:")

	_, err = os.Stat(filepath.Join(workingDir, "pnpm-lock.yaml"))
	if os.IsNotExist(err) {
		ip.logger.Action("pnpm-lock.yaml -> Not found")
		ip.logger.Break()
		return true, "", nil
	} else if err != nil {

		return true, "", fmt.Errorf("unable to read pnpm-lock.yaml file: %w", err)
	}

	ip.logger.Action("pnpm-lock.yaml -> Found")
	ip.logger.Break()

	buffer := bytes.NewBuffer(nil)
	err = ip.executable.Execute(pexec.Execution{
		Args:   []string{"config", "list"},
		Stdout: buffer,
		Stderr: buffer,
		Dir:    workingDir,
	})
	if err != nil {
		return true, "", fmt.Errorf("failed to execute pnpm config output:\n%s\nerror: %s", buffer.String(), err)
	}

	nodeEnv := os.Getenv("NODE_ENV")
	buffer.WriteString(nodeEnv)

	file, err := os.CreateTemp("", "config-file")
	if err != nil {
		return true, "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if closeFileErr := file.Close(); closeFileErr != nil && err == nil {
			err = fmt.Errorf("failed to close temp file: %w", closeFileErr)
		}
	}()

	_, err = file.Write(buffer.Bytes())
	if err != nil {
		return true, "", fmt.Errorf("failed to write temp file for %s: %w", file.Name(), err)
	}

	sum, err := ip.summer.Sum(filepath.Join(workingDir, "pnpm-lock.yaml"), filepath.Join(workingDir, "package.json"), file.Name())
	if err != nil {
		return true, "", fmt.Errorf("unable to sum config files: %w", err)
	}

	prevSHA, ok := metadata["cache_sha"].(string)
	if (ok && sum != prevSHA) || !ok {
		return true, sum, nil
	}

	return false, "", nil
}

func (ip PnpmInstallProcess) SetupModules(workingDir string) error {
	file, err := os.Lstat(filepath.Join(workingDir, "node_modules"))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to stat node_modules directory: %w", err)
		}

	}

	if file != nil && file.Mode()&os.ModeSymlink == os.ModeSymlink {
		err = os.RemoveAll(filepath.Join(workingDir, "node_modules"))
		if err != nil {
			//not tested
			return fmt.Errorf("failed to remove node_modules symlink: %w", err)
		}
	}
	return nil

}

// The build process here relies on pnpm install ... --frozen-lockfile note that
// even if we provide a node_modules directory we must run a 'pnpm install' as
// this is the ONLY way to rebuild native extensions.
func (ip PnpmInstallProcess) Execute(workingDir, modulesLayerPath, storeDir string, launch bool) error {
	environment := os.Environ()
	environment = append(environment, fmt.Sprintf("PATH=%s%c%s", os.Getenv("PATH"), os.PathListSeparator, filepath.Join("node_modules", ".bin")))

	environment = append(environment, "CI=true")

	buffer := bytes.NewBuffer(nil)

	err := ip.executable.Execute(pexec.Execution{
		Args:   []string{"store", "path"},
		Stdout: buffer,
		Stderr: buffer,
		Env:    environment,
		Dir:    workingDir,
	})
	if err != nil {
		return fmt.Errorf("failed to execute pnpm config output:\n%s\nerror: %s", buffer.String(), err)
	}

	installArgs := []string{"install", "--frozen-lockfile"}

	if !launch {
		installArgs = append(installArgs, "--prod", "false")
	}

	storeDirPath := buffer.String()

	info, err := os.Stat(storeDirPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to confirm existence of store directory: %w", err)
	}

	if info != nil && info.IsDir() {
		installArgs = append(installArgs, "--offline")
	}

	installArgs = append(installArgs, "--store-dir", storeDir)

	ip.logger.Subprocess("Running 'pnpm %s'", strings.Join(installArgs, " "))

	err = ip.executable.Execute(pexec.Execution{
		Args:   installArgs,
		Env:    environment,
		Stdout: ip.logger.ActionWriter,
		Stderr: ip.logger.ActionWriter,
		Dir:    workingDir,
	})
	if err != nil {
		return fmt.Errorf("failed to execute pnpm install: %w", err)
	}

	// 'modules-dir' is not working as expected in pnpm (see: https://github.com/pnpm/pnpm/issues/5800),
	// so we leave node_modules in the workspace then move back to the modulesLayerPath.
	err = fs.Move(filepath.Join(workingDir, "node_modules"), filepath.Join(modulesLayerPath, "node_modules"))
	if err != nil {
		return err
	}

	return nil
}
