/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/bf2fc6cc711aee1a0c2a/autoheal-operator/api/v1alpha1"
	log "github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// AutohealConfigReconciler reconciles a AutohealConfig object
type AutohealConfigReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	httpClient http.Client
}

const (
	RequeueDelaySuccess     = 10 * time.Second
	RequeueDelayError       = 5 * time.Second
	AutohealConfigfinalizer = "autoheal-deletion"
)

//+kubebuilder:rbac:groups=autoheal.redhat.com,resources=autohealconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=autoheal.redhat.com,resources=autohealconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=autoheal.redhat.com,resources=autohealconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AutohealConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *AutohealConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	autoheal := &v1alpha1.AutohealConfig{}
	err := r.Client.Get(ctx, req.NamespacedName, autoheal)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Infof("Autoheal CR not found%s", err)
			return ctrl.Result{}, nil
		}
		log.Errorf("Error getting Autoheal CR %s", err)
		return ctrl.Result{}, err
	}
	if autoheal.DeletionTimestamp == nil && autoheal.Finalizers == nil {
		autoheal.Finalizers = append(autoheal.Finalizers, AutohealConfigfinalizer)
		err = r.Update(ctx, autoheal)
		if err != nil {
			log.Errorf("Unable to add finalizer %s", err)
			return ctrl.Result{}, err
		}
	}

	if autoheal.DeletionTimestamp == nil {
		if autoheal.Status.LastSyncPeriod != 0 {
			lastSync := time.Unix(autoheal.Status.LastSyncPeriod, 0)
			period, err := time.ParseDuration(autoheal.Spec.SyncPeriod)
			if autoheal.Spec.SyncPeriod == "" {
				log.Info("Please specify a sync period")
			}
			if err != nil {
				log.Errorf("Error parsing operator resync period %s", err)
				return ctrl.Result{}, err
			}
			nextSync := lastSync.Add(period)
			if time.Now().Before(nextSync) {
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: RequeueDelaySuccess,
				}, nil
			}
		}

		configRepo, err := r.getConfigRepoSecret(ctx, autoheal, &autoheal.Status)
		if err != nil {
			log.Errorf("Unable to reconcile getting config repo secret %s", err)
		}

		if configRepo != nil {
			files, err := r.readFiles(configRepo)
			if err != nil {
				log.Errorf("Unable to get file endpoints from config repo %s", err)
			}
			scenarios, err := r.getScenarios(files, configRepo, autoheal)
			if err != nil {
				log.Errorf("Unable to get scenerios from config repo %s", err)
			}
			err = r.createConfigMaps(autoheal, scenarios, ctx)
			if err != nil {
				log.Errorf("Unable to create configmaps %s", err)
			}
			err = r.removeUnwantedConfigmap(ctx, &scenarios)
			if err != nil {
				log.Error("Unable to delete unwanted configmaps")
			}
		}
	} else {
		err = r.deleteConfigmap(autoheal, ctx)
		if err != nil {
			log.Errorf("Unable to reconcile deleting config maps %s", err)
		}
		if autoheal.Finalizers != nil {
			autoheal.Finalizers = []string{}
			err = r.Update(ctx, autoheal)
			if err != nil {
				log.Errorf("Unable to update finalizer to remove it %s", err)
				return ctrl.Result{}, err
			}
		}
	}
	return r.updateStatus(autoheal)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AutohealConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AutohealConfig{}).
		Complete(r)
}

// GetConfigRepoSecret Gets the secret needed to provide credentials to sycn with github repo
func (r *AutohealConfigReconciler) getConfigRepoSecret(ctx context.Context, autoheal *v1alpha1.AutohealConfig, status *v1alpha1.AutohealConfigStatus) (*v1alpha1.ConfigRepositoryInfo, error) {
	log.Info("Stage: Reading config secret")

	configRepoSecret := &core.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: autoheal.Spec.SecretName, Namespace: autoheal.Namespace}, configRepoSecret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info(err)
			return nil, nil
		}
		log.Errorf("Can't get config repo secret %s", err)
		return nil, err
	}

	if string(configRepoSecret.Data["repository"]) == "" || string(configRepoSecret.Data["tag"]) == "" || string(configRepoSecret.Data["access_token"]) == "" {
		log.Info("Config secret found, one or more of the values are empty")
		return nil, nil
	}

	configurl := string(configRepoSecret.Data["repository"])
	_, err = url.ParseRequestURI(configurl)
	if err != nil {
		log.Errorf("Unable to parse config URL from secret %s", err)
		return nil, err
	}

	var configRepoInfo v1alpha1.ConfigRepositoryInfo
	configRepoInfo.Tag = string(configRepoSecret.Data["tag"])
	configRepoInfo.RepositoryURL = string(configRepoSecret.Data["repository"])
	configRepoInfo.AccessToken = string(configRepoSecret.Data["access_token"])

	status.LastSyncPeriod = time.Now().Unix()
	return &configRepoInfo, nil
}

// ReadFiles Gets all the filename of the scripts from the repo
func (r *AutohealConfigReconciler) readFiles(repo *v1alpha1.ConfigRepositoryInfo) (*v1alpha1.AutohealConfigTree, error) {
	log.Info("Stage: Parsing file endpoints")
	repoUrl, err := url.ParseRequestURI(fmt.Sprintf("%s/git/trees/%s?recursive=true", repo.RepositoryURL, repo.Tag))

	if err != nil {
		log.Errorf("Unable to parse url %s", err)
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, repoUrl.String(), nil)
	if err != nil {
		log.Errorf("Unable to create new http request%s", err)
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("token %s", repo.AccessToken))
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		log.Errorf("Unable to send http request %s", err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		log.Infof("Non 200 response back, got status code %d ", resp.StatusCode)
		return nil, nil
	}

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Unable to read http response body %s", err)
		return nil, err
	}

	err = resp.Body.Close()
	if err != nil {
		return nil, err
	}

	fileName := v1alpha1.AutohealConfigTree{}
	files := v1alpha1.AutohealConfigTree{}

	err = yaml.Unmarshal(bytes, &fileName)
	if err != nil {
		log.Errorf("Unable to parse yaml file endpoints from config repo %s", err)
		return nil, err
	}

	for _, endpoints := range fileName.Tree {
		if strings.Contains(endpoints.Filename, ".yaml") {
			files.Tree = append(files.Tree, endpoints)
		}
	}
	return &files, nil
}

// getScenarios Retrieves the scenarios scripts from the files gotten from the file names in the above function
func (r *AutohealConfigReconciler) getScenarios(files *v1alpha1.AutohealConfigTree, repo *v1alpha1.ConfigRepositoryInfo, autoheal *v1alpha1.AutohealConfig) ([]v1alpha1.AutohealConfigScenario, error) {
	log.Info("Stage: Retrieving Scenarios from config repo")

	scenario := v1alpha1.AutohealConfigScenario{}
	scenarios := []v1alpha1.AutohealConfigScenario{}

	for _, endpoints := range files.Tree {
		repoUrl, err := url.ParseRequestURI(fmt.Sprintf("%s/contents/%s", repo.RepositoryURL, endpoints.Filename))
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest(http.MethodGet, repoUrl.String(), nil)
		if err != nil {
			log.Errorf("Unable to create new http request")
			return nil, err
		}

		if repo.Tag != "" {
			q := req.URL.Query()
			q.Add("ref", repo.Tag)
			req.URL.RawQuery = q.Encode()
		}

		req.Header.Set("Authorization", fmt.Sprintf("token %s", repo.AccessToken))
		req.Header.Set("Accept", "application/vnd.github.v3.raw")

		resp, err := r.httpClient.Do(req)
		if err != nil {
			log.Errorf("Unable to send http request")
			return nil, err

		}

		if resp.StatusCode != http.StatusOK {
			log.Infof("Non 200 response back, got status code %d ", resp.StatusCode)
			return nil, nil
		}

		bytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Errorf("Unable to read response body %s", err)
			return nil, err

		}

		err = resp.Body.Close()
		if err != nil {
			return nil, err
		}

		err = yaml.Unmarshal(bytes, &scenario)
		if err != nil {
			log.Errorf("Unable to parse yaml scenarios from config repo %s", err)
			return nil, err

		}

		scenarios = append(scenarios, scenario)

	}
	return scenarios, nil
}

// createConfigMaps Creates the config maps based on the scenarios gotten from github
func (r *AutohealConfigReconciler) createConfigMaps(autoheal *v1alpha1.AutohealConfig, scenario []v1alpha1.AutohealConfigScenario, ctx context.Context) error {
	log.Info("Stage: Creating/updating configmaps")

	for _, scenarios := range scenario {
		configmap := core.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scenarios.Name,
				Namespace: autoheal.Namespace,
			},
		}

		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, &configmap, func() error {
			if configmap.Data == nil {
				configmap.Data = make(map[string]string)
			}

			configmap.Labels = map[string]string{
				"app": "autoheal",
			}

			configmap.Data["query"] = scenarios.Query
			configmap.Data["label"] = scenarios.Label
			configmap.Data["script"] = scenarios.Script
			configmap.Data["gracePeriod"] = scenarios.GracePeriod
			return nil

		})
		if err != nil {
			log.Errorf("Unable to create configmap")
			return err
		}
	}
	return nil
}

// updateStatus Updates the controller to how long until next requeue of the reconciller depening on if its a error or not
func (r *AutohealConfigReconciler) updateStatus(autoheal *v1alpha1.AutohealConfig) (ctrl.Result, error) {
	err := r.Client.Status().Update(context.Background(), autoheal)

	if err != nil {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: RequeueDelayError,
		}, err
	}

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: RequeueDelaySuccess,
	}, nil
}


// deleteConfigmap Deletes all configs for cleanup
func (r *AutohealConfigReconciler) deleteConfigmap(autoheal *v1alpha1.AutohealConfig, ctx context.Context) error {
	configmap := &core.ConfigMapList{}
	configSelector := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app": "autoheal",
		}),
		Namespace: autoheal.Namespace,
	}

	err := r.Client.List(ctx, configmap, configSelector)
	if err != nil {
		log.Errorf("Failed to get list of configmaps")
		return err
	}

	log.Info("Stage Clean up: Deleting configmap")

	for _, configs := range configmap.Items {
		err = r.Client.Delete(ctx, &configs)
		if err != nil {
			log.Errorf("Error cannnot delete configmap")
			return err
		}
	}
	return nil
}

// removeUnwantedConfigmap Deletes unwanted config maps i.e if the config repo has been changed to remove a scenerio it is removed from the cluster
func (r *AutohealConfigReconciler) removeUnwantedConfigmap(ctx context.Context, scenarios *[]v1alpha1.AutohealConfigScenario) error {
	configmap := &core.ConfigMapList{}
	configSelector := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app": "autoheal",
		}),
	}

	err := r.Client.List(ctx, configmap, configSelector)
	if err != nil {
		log.Errorf("Error cant get configmap %s", err)
		return err
	}

	isRequested := func(name string) bool {
		for _, files := range *scenarios {

			if name == files.Name {
				return true
			}
		}
		return false
	}

	for _, configs := range configmap.Items {

		if !isRequested(configs.Name) {
			log.Infof("Deleting unwanted configmap name: %s", configs.Name)
			err = r.Client.Delete(ctx, &configs)
			if err != nil {
				if apierrors.IsNotFound(err) {
					log.Info("Cannot find configmap:", configs)
				}
			}
		}
	}
	return nil

}
