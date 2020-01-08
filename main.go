package main

import (
	"flag"
	"log"
	"math/rand"
	"time"

	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

var (
	minWatchTimeout = 5 * time.Minute
)

func main() {
	masterURL := flag.String("master", "", "kubernetes api server url")
	kubeconfigPath := flag.String("kubeconfig", "", "path to kubeconfig file")
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfigPath)
	if err != nil {
		log.Fatalln(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln(err)
	}

	nsCh := make(chan *v1.Namespace, 128)

	go nsWatcher(clientset, nsCh)

	for ns := range nsCh {
		err := handleNs(clientset, ns)
		if err != nil {
			log.Println(err)
		}
	}
}

func nsWatcher(clientset *kubernetes.Clientset, c chan *v1.Namespace) {
	for {
		err := internalNsWatcher(clientset, c)
		if statusErr, ok := err.(*apierrs.StatusError); ok {
			if statusErr.ErrStatus.Reason == metav1.StatusReasonExpired {
				log.Println("nsWatcher:", err, "Restarting watch")
				continue
			}
		}

		log.Fatalln(err)
	}
}

func internalNsWatcher(clientset *kubernetes.Clientset, c chan *v1.Namespace) error {
	list, err := clientset.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	resourceVersion := list.ResourceVersion

	for {
		log.Println("nsWatcher: watching since", resourceVersion)

		timeoutSeconds := int64(minWatchTimeout.Seconds() * (rand.Float64() + 1.0))
		watcher, err := clientset.CoreV1().Namespaces().Watch(metav1.ListOptions{
			ResourceVersion: resourceVersion,
			TimeoutSeconds:  &timeoutSeconds,
		})
		if err != nil {
			return err
		}

		for watchEvent := range watcher.ResultChan() {
			if watchEvent.Type == watch.Error {
				return apierrs.FromObject(watchEvent.Object)
			}

			ns, ok := watchEvent.Object.(*v1.Namespace)
			if !ok {
				log.Println("nsWatcher: unexpected kind:", watchEvent.Object.GetObjectKind().GroupVersionKind())
				continue
			}

			resourceVersion = ns.ResourceVersion

			if watchEvent.Type != watch.Added {
				continue
			}

			c <- ns
		}
	}
}

func handleNs(clientset *kubernetes.Clientset, ns *v1.Namespace) error {
	log.Println("Creating default network policy in namespace", ns.Name)

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				networkingv1.NetworkPolicyIngressRule{
					From: []networkingv1.NetworkPolicyPeer{
						networkingv1.NetworkPolicyPeer{
							PodSelector: &metav1.LabelSelector{},
						},
					},
				},
			},
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	c := clientset.NetworkingV1().NetworkPolicies(ns.Name)
	_, err := c.Create(policy)
	return err
}
