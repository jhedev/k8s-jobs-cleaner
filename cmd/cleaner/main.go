package main

import (
	"flag"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	ignoreAnnotation             = "jhe.io/ignore"
	deleteAfterSecondsAnnotation = "jhe.io/delete-after-seconds"
)

var (
	defaultDeleteAfterSeconds = int(time.Hour.Seconds())
)

func main() {
	var (
		logger     = log.New()
		inCluster  = flag.Bool("in-cluster", false, "runs in cluster")
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")

		config *rest.Config
		err    error
	)
	flag.Parse()

	if *inCluster {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			logger.Fatal(err)
		}
	} else {
		// use the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			logger.Fatal(err)
		}
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Fatal(err.Error())
	}

	jobs, err := clientset.BatchV1().Jobs("").List(metav1.ListOptions{})
	if err != nil {
		logger.Fatal(err.Error())
	}

	for _, job := range jobs.Items {
		name := job.Name
		l := logger.WithFields(log.Fields{
			"job": name,
		})

		completion := job.Status.CompletionTime
		if completion == nil {
			l.Infof("job not yet finished. skipping...", name)
			continue
		}
		if ignore, ok := job.Annotations[ignoreAnnotation]; ok && strings.ToLower(ignore) == "true" {
			l.Infof("job is marked to ignore skipping")
			continue
		}

		secs := defaultDeleteAfterSeconds
		if delAfterSecs, ok := job.Annotations[deleteAfterSecondsAnnotation]; ok {
			secs, err = strconv.Atoi(delAfterSecs)
			if err != nil {
				l.Error("cannot convert seconds value: %+v. Skipping...", err)
			}
		}

		if completion.Time.Add(time.Duration(secs) * time.Second).Before(time.Now()) {
			l.Infof("job is ready for deletion. Deleting...")
			if err := clientset.BatchV1().Jobs(job.Namespace).Delete(name, nil); err != nil {
				l.Errorf("delete: %+v", err)
			}
		}
	}
}
