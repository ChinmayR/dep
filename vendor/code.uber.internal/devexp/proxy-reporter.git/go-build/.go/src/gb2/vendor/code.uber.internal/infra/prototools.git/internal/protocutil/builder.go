package protocutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type builder struct {
	GoPlugin       string
	AddGRPC        bool
	ExtraGoPlugins []string
	ProtocPath     string
	CmdRunner      func(*exec.Cmd) error
	LogFunc        func(string, ...interface{})
}

func newBuilder(options ...BuilderOption) *builder {
	builder := &builder{
		GoPlugin:   DefaultGoPlugin,
		AddGRPC:    DefaultAddGRPC,
		ProtocPath: DefaultProtocPath,
		CmdRunner:  (*exec.Cmd).Run,
		LogFunc:    func(string, ...interface{}) {},
	}
	for _, option := range options {
		option(builder)
	}
	return builder
}

func (b *builder) Build(projectRoot string, inputDirPath string, outputDirPath string, options ...BuildOption) error {
	buildOptions := newBuildOptions(options...)
	relProtoFilePaths, err := getAllRelProtoFilePaths(inputDirPath)
	if err != nil {
		return err
	}
	relProtoFilePaths, err = filterFilePaths(relProtoFilePaths, buildOptions.ExcludePatterns)
	if err != nil {
		return err
	}
	relDirPathToProtoFiles := getRelDirPathToFiles(relProtoFilePaths)
	cmds, err := b.getProtocCmds(relDirPathToProtoFiles, projectRoot, inputDirPath, outputDirPath, buildOptions.ExtraIncludes)
	if err != nil {
		return err
	}
	for _, cmd := range cmds {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		b.LogFunc(strings.Join(cmd.Args, " "))
		if err := b.CmdRunner(cmd); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) getProtocCmds(relDirPathToProtoFiles map[string][]string, projectRoot string, inputDirPath string, outputDirPath string, extraIncludes []string) ([]*exec.Cmd, error) {
	var cmds []*exec.Cmd
	for relDirPath, protoFiles := range relDirPathToProtoFiles {
		fileList := make([]string, 0, len(protoFiles))
		for _, protoFile := range protoFiles {
			fileList = append(fileList, filepath.Join(inputDirPath, relDirPath, protoFile))
		}

		modifiers := getModifiers(relDirPathToProtoFiles, relDirPath, projectRoot, outputDirPath)
		goOutOpts := modifiers
		if b.AddGRPC {
			if goOutOpts != "" {
				goOutOpts = fmt.Sprintf("%s,", goOutOpts)
			}
			goOutOpts = fmt.Sprintf("%splugins=grpc", goOutOpts)
		}

		args := []string{"-I", inputDirPath}
		for _, extraInclude := range extraIncludes {
			args = append(args, "-I", extraInclude)
		}
		if goOutOpts == "" {
			args = append(args, fmt.Sprintf("--%s_out=%s", b.GoPlugin, outputDirPath))
		} else {
			args = append(args, fmt.Sprintf("--%s_out=%s:%s", b.GoPlugin, goOutOpts, outputDirPath))
		}

		for _, extraGoPlugin := range b.ExtraGoPlugins {
			if modifiers == "" {
				args = append(args, fmt.Sprintf("--%s_out=%s", extraGoPlugin, outputDirPath))
			} else {
				args = append(args, fmt.Sprintf("--%s_out=%s:%s", extraGoPlugin, modifiers, outputDirPath))
			}
		}
		cmds = append(cmds, exec.Command(b.ProtocPath, append(args, fileList...)...))
	}
	return cmds, nil
}

func getAllRelProtoFilePaths(dirPath string) ([]string, error) {
	var relProtoFilePaths []string
	if err := filepath.Walk(
		dirPath,
		func(filePath string, fileInfo os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fileInfo.IsDir() {
				return nil
			}
			if filepath.Ext(filePath) != ".proto" {
				return nil
			}
			relFilePath, err := filepath.Rel(dirPath, filePath)
			if err != nil {
				return err
			}
			relProtoFilePaths = append(relProtoFilePaths, relFilePath)
			return nil
		},
	); err != nil {
		return nil, err
	}
	return relProtoFilePaths, nil
}

func getRelDirPathToFiles(relFilePaths []string) map[string][]string {
	relDirPathToFiles := make(map[string][]string)
	for _, relFilePath := range relFilePaths {
		relDirPath := filepath.Dir(relFilePath)
		if _, ok := relDirPathToFiles[relDirPath]; !ok {
			relDirPathToFiles[relDirPath] = make([]string, 0)
		}
		relDirPathToFiles[relDirPath] = append(relDirPathToFiles[relDirPath], filepath.Base(relFilePath))
	}
	return relDirPathToFiles
}

func filterFilePaths(filePaths []string, excludeFilePatterns []string) ([]string, error) {
	var filteredFilePaths []string
	for _, filePath := range filePaths {
		matched, err := filePathMatches(filePath, excludeFilePatterns)
		if err != nil {
			return nil, err
		}
		if !matched {
			filteredFilePaths = append(filteredFilePaths, filePath)
		}
	}
	return filteredFilePaths, nil
}

func filePathMatches(filePath string, excludeFilePatterns []string) (bool, error) {
	for _, excludeFilePattern := range excludeFilePatterns {
		if strings.HasPrefix(filePath, excludeFilePattern) {
			return true, nil
		}
		matched, err := filepath.Match(excludeFilePattern, filePath)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func getModifiers(relDirPathToProtoFiles map[string][]string, curRelDirPath string, projectRoot string, outputDirPath string) string {
	modifiers := make(map[string]string, 0)
	for relDirPath, protoFiles := range relDirPathToProtoFiles {
		if relDirPath != curRelDirPath {
			for _, protoFile := range protoFiles {
				modifiers[filepath.Join(relDirPath, protoFile)] = filepath.Join(projectRoot, outputDirPath, relDirPath)
			}
		}
	}
	// this is just so the command line is easier to read and deterministic
	modifierKeys := make([]string, len(modifiers))
	i := 0
	for key := range modifiers {
		modifierKeys[i] = key
		i++
	}
	sort.Strings(modifierKeys)
	modifierStrings := make([]string, len(modifierKeys))
	for i, modifierKey := range modifierKeys {
		modifierStrings[i] = fmt.Sprintf("M%s=%s", modifierKey, modifiers[modifierKey])
	}
	return strings.Join(modifierStrings, ",")
}
