// Copyright [2016] [Matthew Stump, Vorstella Corp.]

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	metav1 "k8s.io/client-go/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"os"
	"os/exec"
	"regexp"
)

const (
	AnnotationsKey  string = "annotations"
	MetaDataKey     string = "metadata"
	NodetoolCommand string = "info"
	UuidRegex       string = "[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}"
)

var (
	Trace   *log.Logger
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

func initLogging(
	traceHandle io.Writer,
	infoHandle io.Writer,
	warningHandle io.Writer,
	errorHandle io.Writer) {

	Trace = log.New(traceHandle,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(infoHandle,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Warning = log.New(warningHandle,
		"WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Error = log.New(errorHandle,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}

func main() {
	initLogging(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr)
	kubeConfigPath := flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	nodetoolPath := flag.String("nodetool", "/usr/bin/nodetool", "path to cassandra nodetool")
	namespace := flag.String("namespace", "default", "the kubernetes namespace")
	podName := flag.String("pod", "", "the kubernetes pod name")
	populateHostId := flag.Bool("populate", true, "populate the k8s annotations with our host ID")
	fetchHostId := flag.Bool("fetch", true, "fetch our host ID from k8s annotations")
	annotationsPrefix := flag.String("prefix", "cassandra", "the prefix for the annotations tracking host IDs")
	flag.Parse()

	if len(*podName) == 0 {
		panic(errors.New("you must specify a pod name"))
	}

	var kubeConfig *rest.Config
	var err error

	if len(*kubeConfigPath) == 0 {
		kubeConfig, err = rest.InClusterConfig()
	} else {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", *kubeConfigPath)
	}
	if err != nil {
		panic(err.Error())
	}

	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		Error.Println("Could not open k8s intra-cluster config")
		panic(err.Error())
	}

	pods := client.Pods(*namespace)
	pod, err := pods.Get(*podName, metav1.GetOptions{})
	if err != nil {
		Error.Println("Could not find our pod:", *podName)
		panic(err.Error())
	}

	annotations := pod.ObjectMeta.Annotations
	createdByJson, ok := annotations[api.CreatedByAnnotation]
	if !ok {
		panic(errors.New("the created-by annotation isn't present"))
	}

	var createdBySerialized v1.SerializedReference
	err = json.Unmarshal([]byte(createdByJson), &createdBySerialized)
	if err != nil {
		panic(err.Error())
	}

	createdBy := createdBySerialized.Reference

	if *populateHostId {
		err := PopulateHostId(client, *nodetoolPath, createdBy.Namespace, createdBy.Name, *annotationsPrefix, *podName)
		if err != nil {
			panic(err.Error())
		}
	}

	if *fetchHostId {
		hostId, err := FetchHostId(client, createdBy.Namespace, createdBy.Name, *annotationsPrefix, *podName)
		if err != nil {
			panic(err.Error())
		}
		fmt.Println(hostId)
	}
}

func FetchHostId(client *kubernetes.Clientset, namespace string, createdBy string, annotationsPrefix string, podName string) (string, error) {

	statefulSets := client.StatefulSets(namespace)
	statefulSet, err := statefulSets.Get(createdBy, metav1.GetOptions{})
	if err != nil {
		Error.Println("Could not find StatefulSet", createdBy)
		return "", err
	}

	hostId, ok := statefulSet.ObjectMeta.Annotations[getAnnotationName(annotationsPrefix, podName)]
	if !ok {
		return "", errors.New(fmt.Sprint("Host ID for", podName, "not present in annotations for StatefulSet", createdBy))
	}

	return hostId, nil
}

func PopulateHostId(client *kubernetes.Clientset, nodetoolPath string, namespace string, createdBy string, annotationsPrefix string, podName string) error {

	hostId, err := getCassandraHostId(nodetoolPath)
	if err != nil {
		Error.Println("Could not obtain Cassandra host ID")
		return err
	}

	statefulSets := client.StatefulSets(namespace)
	statefulSet, err := statefulSets.Get(createdBy, metav1.GetOptions{})
	if err != nil {
		Error.Println("Could not find StatefulSet", createdBy)
		return err
	}

	patch := make(map[string]map[string]map[string]string)
	annotationsPatch := make(map[string]map[string]string)
	annotationsPatch[AnnotationsKey] = map[string]string{getAnnotationName(annotationsPrefix, podName): hostId}
	patch[MetaDataKey] = annotationsPatch
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		Error.Println("Could not generate patch JSON")
		return err
	}

	Trace.Println("Patching StatefulSet:", string(patchBytes))
	result, err := statefulSets.Patch(statefulSet.Name, api.MergePatchType, patchBytes)
	if err != nil {
		Error.Println("Error patching StatefulSet")
		return err
	}

	Trace.Println("Resulting annotations:", result.ObjectMeta.Annotations)
	return nil
}

func getAnnotationName(prefix string, name string) string {
	return fmt.Sprintf("%v/%v", prefix, name)
}

func getCassandraHostId(nodetool string) (string, error) {
	nodetoolOutput, err := runCommand(nodetool, NodetoolCommand)
	if err != nil {
		return "", err
	}
	uuidRegex, _ := regexp.Compile(UuidRegex)
	match := uuidRegex.FindString(nodetoolOutput)
	if len(match) == 0 {
		return "", errors.New("couldn't fetch Cassandra host ID")
	}
	return match, nil
}

func runCommand(name string, arg ...string) (string, error) {
	cmd := exec.Command(name, arg...)
	var stdOut bytes.Buffer
	cmd.Stdout = &stdOut
	err := cmd.Run()
	if err != nil {
		return stdOut.String(), err
	}
	return stdOut.String(), nil
}
