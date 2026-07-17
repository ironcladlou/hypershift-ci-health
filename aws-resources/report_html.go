package main

import (
	_ "embed"
	"io"
)

//go:embed report.html
var htmlPage string

func PrintHTML(w io.Writer) error {
	_, err := io.WriteString(w, htmlPage)
	return err
}
