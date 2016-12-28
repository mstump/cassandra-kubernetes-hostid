# What is this?

This tool is used to both populate and read the Cassandra host ID from the annotations of the parent StatefulSet (PetSet) in Kubernetes.

# Motivation

The best practices for Cassandra state that you should use local storage physically attached to the machine. However, if this machine fails and data is lost then you'll need to provision a replacement node. The method for doing this is to pass the IP or host UUID via the JVM flag: ```-Dcassandra.replace_address=e2a79390-c458-4387-8034-c14da6d38a22```. Because IPs in Kubernetes are not stable the host ID is the only way to retain identity after data loss. This tool will persist or read the host ID using the annotations of the Kubernetes API.

# Why use StatefulSet instead of Pod annotations?
The preferred deployment methodology for Cassandra on Kubernetes is to use StatefulSets (PetSet). Currently the method for upgrading containers in a StatefulSet is to delete each Pod and allow Kubernetes to recreate the Pod using the new container. However, when you delete a Pod the metadata for the Pod is lost. If we persist the data to the parent StatefulSet we can retain data across upgrades.

# Example

```
./cassandra-kubernetes --fetch --pod cassandra-0
e2a79390-c458-4387-8034-c14da6d38a22
```

# Usage

```
Usage of cassandra-kubernetes:
  -alsologtostderr
    	log to standard error as well as files
  -kubeconfig string
    	absolute path to the kubeconfig file
  -log_backtrace_at value
    	when logging hits line file:N, emit a stack trace
  -log_dir string
    	If non-empty, write log files in this directory
  -logtostderr
    	log to standard error instead of files
  -namespace string
    	the kubernetes namespace (default "default")
  -pod string
    	the kubernetes pod name
  -prefix string
    	the prefix for the annotations tracking host IDs (default "cassandra")
  -stderrthreshold value
    	logs at or above this threshold go to stderr
  -v value
    	log level for V logs
  -vmodule value
    	comma-separated list of pattern=N settings for file-filtered logging
```
