package main

import (
	"fmt"
	"sort"

	"github.com/gotd/getdoc"
)

func main() {
	doc, err := getdoc.Load(getdoc.LayerLatest)
	if err != nil {
		panic(err)
	}

	var method []string
	for _, m := range doc.Methods {
		if !m.BotCanUse {
			continue
		}
		method = append(method, m.Name)
	}

	sort.Strings(method)
	for _, m := range method {
		fmt.Println(m)
	}
}
