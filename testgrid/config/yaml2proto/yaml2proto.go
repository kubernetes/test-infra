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
