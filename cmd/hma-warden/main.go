package main

import (
	"context"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	podToAddr map[string]string
)

func mustFetchNamespaces(ctx context.Context, clientset *kubernetes.Clientset) []string {
	// watch events of all deployments
	namespaces, e := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if e != nil {
		log.Fatal("failed to fetch namespaces: ", e)
	}
	var result []string
	for _, ns := range namespaces.Items {
		result = append(result, ns.Name)
	}
	return result
}

func mustFetchPods(ctx context.Context, ns string, clientset *kubernetes.Clientset) []string {
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	pods, e := clientset.CoreV1().Pods(ns).List(timeoutCtx, metav1.ListOptions{})
	if e != nil {
		log.Fatal(e)
	}

	var result []string
	for _, pod := range pods.Items {
		result = append(result, pod.Name)
	}
	return result
}

func registerPods(ctx context.Context, clientset *kubernetes.Clientset) {
	nss := mustFetchNamespaces(ctx, clientset)
	for _, ns := range nss {
		pods := mustFetchPods(ctx, ns, clientset)
		for _, pod := range pods {
			podToAddr[pod] = ""
		}
	}
}

func doAddressTranslationForPod(ctx context.Context, ns, podName string, clientset *kubernetes.Clientset) error {
	clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	return nil
}

func watchNamespaces(ctx context.Context, clientset *kubernetes.Clientset) {
	timeout := int64(60)
	watcher, _ := clientset.CoreV1().Namespaces().Watch(ctx, metav1.ListOptions{TimeoutSeconds: &timeout})

	for event := range watcher.ResultChan() {
		item := event.Object.(*corev1.Namespace)

		switch event.Type {
		case watch.Modified:
		case watch.Bookmark:
		case watch.Error:
		case watch.Deleted:
		case watch.Added:
			processNamespace(item.GetName(), event.Type)
		}
	}
}

func watchServices(ctx context.Context, ns string, clientset *kubernetes.Clientset) {
	timeout := int64(60)
	watcher, _ := clientset.CoreV1().Services(ns).Watch(ctx, metav1.ListOptions{TimeoutSeconds: &timeout})

	for event := range watcher.ResultChan() {
		item := event.Object.(*corev1.Service)

		switch event.Type {
		case watch.Modified:
		case watch.Bookmark:
		case watch.Error:
		case watch.Deleted:
		case watch.Added:
			processService(item.GetName(), event.Type)
		}
	}
}

func processNamespace(ns string, event watch.EventType) {
	log.Infof("got event %s on namespace %s", event, ns)
}

func processService(svc string, event watch.EventType) {
	log.Infof("got event %s on service %s", event, svc)
}

func main() {
	ctx := context.Background()
	config, e := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if e != nil {
		panic(e)
	}
	log.Info("initialized config")
	clientset, e := kubernetes.NewForConfig(config)
	if e != nil {
		panic(e)
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go watchNamespaces(ctx, clientset)

	// watch events of all deployments
	namespaces, e := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if e != nil {
		log.Error("error occurred while listing namespaces: ", e)
		return
	}
	for _, namespace := range namespaces.Items {
		wg.Add(1)
		go watchServices(ctx, namespace.Name, clientset)
	}

	registerPods(ctx, clientset)
	for _, namespace := range namespaces.Items {
		doAddressTranslationForPod(ctx, namespace.Name, "", clientset)
	}

	wg.Wait()
}
