package main

import (
	"os"
	"fmt"

	yaml2proto "k8s.io/test-infra/testgrid/config/yaml2proto"
)

//
// usage: config <input/path/to/yaml> <output/path/to/proto>
//

func main(){
	args := os.Args[1:]

	if(len(args) != 2){
		fmt.Printf("Wrong Arguments - usage: yaml2proto <input/path/to/yaml> <output/path/to/proto>\n")
	}

	err := yaml2proto.Yaml2Proto(args[0],args[1])
	if err != nil {
		fmt.Printf("Yaml2Proto Error : %v\n", err)
	}
}

