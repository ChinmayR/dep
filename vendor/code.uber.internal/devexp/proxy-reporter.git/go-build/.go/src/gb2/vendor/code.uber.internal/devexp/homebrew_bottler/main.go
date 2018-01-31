package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

var (
	bottleDefs = make(map[string]*BottleDef)
	tarRegexp  = regexp.MustCompile(`([\w]+)\-(v?([\.\-\d]+)(-([\w\d]+))?)\.([a-z_]+)\.bottle\.tar\.gz`)
)

// BottleDef is a representation of a hombrew bottle definition file
type BottleDef struct {
	name    string
	Bottles []Bottle
	Version string
}

// Bottle is a representation of a single bottle
type Bottle struct {
	OSRelease string
	SHASum    string
}

func main() {
	if len(os.Args) != 3 {
		log.Print("Usage: go run bottle_dsl.go TEMPLATE BOTTLE_DIR")
		log.Fatalln("Wrong number of argumentes specified")
	}
	templateFile := os.Args[1]
	bottlePath := os.Args[2]

	var version string
	if cwd, err := os.Getwd(); err != nil {
		log.Fatalln("Got error getting working directory:", err)
	} else {
		version = getVersion(cwd)
	}

	if _, err := os.Stat(templateFile); os.IsNotExist(err) {
		log.Fatalf("Template %s does not exist\n", templateFile)
	}

	if err := filepath.Walk(bottlePath, gzipWalker); err != nil {
		log.Fatalln("Error walking bottle directory:", err.Error())
	}
	log.Printf("Found %d formula(s)\n", len(bottleDefs))

	t := template.Must(template.ParseFiles(templateFile))

	for name, bs := range bottleDefs {
		var buffer bytes.Buffer
		// we only need to get the version once, so it doesn't make sense to set this below
		bs.Version = version
		err := t.Execute(&buffer, bs)
		if err != nil {
			log.Fatalf("Unable to execute template. Bottle Name: %s, error: %s", name, err.Error())
		}
		log.Printf("New bottle definition for %s is:", name)
		fmt.Printf(buffer.String())
		if err := ioutil.WriteFile(fmt.Sprintf("%s.rb", name), buffer.Bytes(), 0666); err != nil {
			log.Fatalln("Error writing bottle file:", err.Error())
		}
	}
}

func gzipWalker(path string, _ os.FileInfo, err error) error {
	if err != nil {
		// if we receive some error from Walk, ignore that path
		log.Printf("Error walking path %s: %s\n", path, err.Error())
		return filepath.SkipDir
	}

	if match, _ := filepath.Match("*.bottle.tar.gz", filepath.Base(path)); match {
		b, err := NewBottleDef(path)
		if err != nil {
			return err
		}
		if bd, ok := bottleDefs[b.name]; ok {
			bd.Bottles = append(bd.Bottles, b.Bottles...)
			return nil
		}
		bottleDefs[b.name] = &b
	}
	return nil
}

// NewBottleDef creates a new BottleDef
func NewBottleDef(path string) (BottleDef, error) {
	var (
		def       BottleDef
		newBottle Bottle
	)

	// shasum time!
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return def, err
	}
	sha := sha256.Sum256(data)
	newBottle.SHASum = hex.EncodeToString(sha[:])

	name, release, err := extractPath(filepath.Base(path))
	if err != nil {
		return def, err
	}
	newBottle.OSRelease = release

	def = BottleDef{
		name: name,
		// third matched part is version
		Bottles: []Bottle{newBottle},
	}
	return def, nil
}

func extractPath(path string) (string, string, error) {
	// fill in the rest of our data
	matches := tarRegexp.FindAllStringSubmatch(path, -1)
	if len(matches) != 1 {
		return "", "", errors.New(fmt.Sprintf("bad bottle path: %s", path))
	}
	parts := matches[0]
	return parts[1], parts[6], nil
}

func getVersion(workspace string) string {
	cmd := exec.Command("git", "describe", "--always", "--dirty")
	cmd.Dir = workspace
	if out, err := cmd.Output(); err != nil {
		log.Fatalf("Error running `git describe` on workspace %s: %s\n", workspace, err.Error())
	} else {
		return strings.TrimSpace(string(out))
	}
	return "" // this will never be hit
}
