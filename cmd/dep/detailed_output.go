package main

import (
	"encoding/json"
	"io"

	"github.com/golang/dep/gps"
)

// TODO: This status was removed during the merge from upstream, its interface methods need to be updated
type DetailedOutput struct {
	w       io.Writer
	basic   []*StripBasicStatus
	missing []*MissingStatus
}

type StripBasicStatus struct {
	ProjectRoot  string
	Children     []string
	Constraint   string
	Version      string
	Revision     string
	Latest       string
	PackageCount int
}

func (out *DetailedOutput) BasicHeader() {
	out.basic = []*StripBasicStatus{}
}

func (out *DetailedOutput) BasicFooter() {
	json.NewEncoder(out.w).Encode(out.basic)
}

func (out *DetailedOutput) BasicLine(bs *BasicStatus) {
	var constraint string
	if v, ok := bs.Constraint.(gps.Version); ok {
		constraint = formatVersion(v)
	} else {
		constraint = ""
	}
	sbs := &StripBasicStatus{
		bs.ProjectRoot,
		bs.Children,
		constraint,
		formatVersion(bs.Version),
		formatVersion(bs.Revision),
		formatVersion(bs.Latest),
		bs.PackageCount,
	}
	out.basic = append(out.basic, sbs)
}

func (out *DetailedOutput) MissingHeader() {
	out.missing = []*MissingStatus{}
}

func (out *DetailedOutput) MissingLine(ms *MissingStatus) {
	out.missing = append(out.missing, ms)
}

func (out *DetailedOutput) MissingFooter() {
	json.NewEncoder(out.w).Encode(out.missing)
}
