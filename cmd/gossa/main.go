package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/bjwbell/ssair"
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
	var logging = flag.Bool("log", false, "enable logging for the ssa package")
	var outf = flag.String("outf", "fn.s", "assembly output file")
	var proto = flag.String("proto", "fn_proto.go", "Go file with fn prototype")
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

	ssafn, ok := ssair.BuildSSA(file, *pkgName, *fn, *logging)
	if ssafn == nil || !ok {
		fmt.Println("Error building SSA form")
		return
	} else {
		fmt.Println("ssa:\n", ssafn)
	}
	_, _, _, fnType, _, err := ssair.TypeCheckFn(file, *pkgName, *fn, *logging)
	if err != nil {
		fmt.Println("ERROR TYPE CHECKING FN")
		return
	}
	_, protoImports, protoFn := ssair.GoProto(fnType)

	if fnProg, ok := ssair.GenProg(ssafn); ok {
		preamble := ssair.Preamble()
		assembly := ssair.Assemble(fnProg)
		fnProto := ssair.FuncProto(ssafn.Name, 0, 0)
		fmt.Println("assembly:")
		fmt.Println(fnProto)
		fmt.Println(assembly)

		out := preamble + "\n" + fnProto + "\n" + assembly
		writeFile(*outf, out)
		protoPkg := "package " + *pkgName
		writeFile(*proto, protoPkg+"\n"+protoImports+"\n"+protoFn)
	} else {
		fmt.Println("Error generating prog from SSA")
		return
	}

}

func writeFile(filename, contents string) {
	if err := ioutil.WriteFile(filename, []byte(contents), 0644); err != nil {
		log.Fatalf("Cannot write to file \"%v\", error \"%v\"\n", filename, err)
	}
}
