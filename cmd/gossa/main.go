package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bjwbell/gossa"
)

func filePath(pathName string) string {
	split := strings.Split(pathName, "/")
	dir := ""
	if len(split) == 1 {
		dir = "."
	} else if len(split) == 2 {
		dir = split[0] + "/"
	} else {
		dir = strings.Join(split[0:len(split)-2], "/")
	}
	return dir
}

func main() {
	var pkgName = flag.String("pkg", "", "input file package name")
	var f = flag.String("f", "", "input file with function definitions")
	var fn = flag.String("fn", "", "function name")
	flag.Parse()

	file := os.ExpandEnv("$GOFILE")
	log.SetFlags(log.Lshortfile)
	if *f != "" {
		file = *f
	}
	if *fn == "" {
		log.Fatalf("Error no function name(s) provided")
	}
	if *pkgName == "" {
		*pkgName = filePath(file)
	}

	ssafn, ok := gossa.ParseSSA(file, *pkgName, *fn)
	if ssafn == nil || !ok {
		fmt.Println("Error building SSA form")
		return
	} else {
		fmt.Println("ssa:\n", ssafn)
	}
	if fnProg, ok := gossa.GenSSA(ssafn); ok {
		assembly := gossa.Assemble(fnProg)
		fmt.Println("assembly:\n", assembly)
	} else {
		fmt.Println("Error generating prog from SSA")
		return
	}

}
