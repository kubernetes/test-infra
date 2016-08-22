/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package yaml2proto

import (
	"fmt"
	"io/ioutil"
	"gopkg.in/yaml.v2"

	"github.com/golang/protobuf/proto"
	pb "k8s.io/test-infra/testgrid/config/pb"
)

func Yaml2Proto(inputFile string, outputFile string) error {

	// Read in yaml file
	data, err := ioutil.ReadFile(inputFile) // yaml datastream
	if err != nil {
		fmt.Printf("ReadFile Error : %v\n", err);
		return err
	}
	
	// Unmarshal yaml to config
	config := &pb.Configuration{}
	err = yaml.Unmarshal(data,&config)
	if err != nil {
		fmt.Printf("Unmarshal Error : %v\n", err);
		return err
	}

	// Marshal config to protobuf
	out, err := proto.Marshal(config)
	if err != nil {
		fmt.Printf("Marshal Error : %v\n", err)
		return err
	}

	// Write out to outputfile
	return ioutil.WriteFile(outputFile,out,0777)
}
