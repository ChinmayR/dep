package main

import (
	"encoding/json"
	"io"

	"github.com/golang/dep/internal/gps"
)

type UberOutput struct {
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

func (out *UberOutput) BasicHeader() {
	out.basic = []*StripBasicStatus{}
}

func (out *UberOutput) BasicFooter() {
	json.NewEncoder(out.w).Encode(out.basic)
}

func (out *UberOutput) BasicLine(bs *BasicStatus) {
	var constraint string
	if v, ok := bs.Constraint.(gps.Version); ok {
		constraint = formatVersion(v)
	} else {
		constraint = bs.Constraint.String()
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

func (out *UberOutput) MissingHeader() {
	out.missing = []*MissingStatus{}
}

func (out *UberOutput) MissingLine(ms *MissingStatus) {
	out.missing = append(out.missing, ms)
}

func (out *UberOutput) MissingFooter() {
	json.NewEncoder(out.w).Encode(out.missing)
}
