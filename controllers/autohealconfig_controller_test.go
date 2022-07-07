package controllers

import (
	// "runtime"
	// "net/http"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"

	// // "k8s.io/apimachinery/pkg/runtime"
	// "k8s.io/kubernetes/pkg/apis/core"
	// "sigs.k8s.io/controller-runtime/pkg/client/fake"
	core "k8s.io/api/core/v1"
	// "sigs.k8s.io/controller-runtime/pkg/client"
	// core "k8s.io/api/core/v1"
	"github.com/bf2fc6cc711aee1a0c2a/autoheal-operator/api/v1alpha1"
	// "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	// "sigs.k8s.io/controller-runtime/pkg/client"
	// "github.com/bf2fc6cc711aee1a0c2a/autoheal-operator/api/v1alpha1"
	// "github.com/bf2fc6cc711aee1a0c2a/autoheal-operator/controllers"
)

var (
	tag         = "test-tag"
	repoUrl     = "http://localhost:8080"
	accessToken = "test-token"
	// sha         = "380580u9ghwr9tq9"
	autoheal    = v1alpha1.AutohealConfig{ObjectMeta: metav1.ObjectMeta{
		Namespace: "autoheal-operator",
		Name:      "autoheal-test",
	},
		Spec: v1alpha1.AutohealConfigSpec{SecretName: "autoheal-config"},
	}
	configRepo = v1alpha1.AutohealConfigTree{

		Tree: []v1alpha1.AutohealConfigTreeResponse{
			{
				Filename: "scenarios/fleetshard/example_broken_issue.yaml",
			},
		},
	}
	configScenario = []v1alpha1.AutohealConfigScenario{
		{
			Name:        "Test-name",
			Query:       "Test query >2",
			Label:       "test label",
			Script:      `println("hello world")`,
			GracePeriod: "2m",
		},
	}
)

func TestGetConfigRepoSecret(t *testing.T) {

	// autoheal := &v1alpha1.AutohealConfig{ObjectMeta: metav1.ObjectMeta{
	// 	Namespace: "autoheal-operator",
	// 	Name:      "autoheal-test",
	// },
	// 	Spec: v1alpha1.AutohealConfigSpec{SecretName: "autoheal-config"},
	// }
	status := &v1alpha1.AutohealConfigStatus{LastSyncPeriod: 50}

	type fields struct {
		Client client.Client
	}

	type args struct {
		ctx      context.Context
		autoheal *v1alpha1.AutohealConfig
		status   *v1alpha1.AutohealConfigStatus
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *v1alpha1.ConfigRepositoryInfo
		err    bool
	}{
		{
			name: "Test secret is present and struct gets populated",
			fields: fields{
				fake.NewClientBuilder().WithRuntimeObjects(&core.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: autoheal.Namespace, Name: autoheal.Spec.SecretName},
					Data: map[string][]byte{
						"tag":          []byte(tag),
						"access_token": []byte(accessToken),
						"repository":   []byte(repoUrl),
					},
				}).Build(),
			},
			args: args{
				ctx:      context.TODO(),
				autoheal: &autoheal,
				status:   status,
			},
			want: &v1alpha1.ConfigRepositoryInfo{
				Tag:           tag,
				AccessToken:   accessToken,
				RepositoryURL: repoUrl,
			},
			err: false,
		},
		{

			name: "Test secret is not present and function returns nothing",
			fields: fields{
				fake.NewClientBuilder().WithRuntimeObjects(&core.Secret{}).Build(),
			},
			args: args{
				ctx:      context.TODO(),
				autoheal: &autoheal,
				status:   status,
			},
			want: nil,
			err:  false,
		},
		{
			name: "Test secret is present but is missing values function returns nothing",
			fields: fields{
				fake.NewClientBuilder().WithRuntimeObjects(&core.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: autoheal.Namespace, Name: autoheal.Spec.SecretName},
					Data: map[string][]byte{
						"tag":          []byte(""),
						"access_token": []byte(accessToken),
						"repository":   []byte(repoUrl),
					},
				}).Build(),
			},
			args: args{
				ctx:      context.TODO(),
				autoheal: &autoheal,
				status:   status,
			},
			want: nil,
			err:  false,
		},
		{
			name: "Test secret is present but url cannot be parsed",
			fields: fields{
				fake.NewClientBuilder().WithRuntimeObjects(&core.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: autoheal.Namespace, Name: autoheal.Spec.SecretName},
					Data: map[string][]byte{
						"tag":          []byte(tag),
						"access_token": []byte(accessToken),
						"repository":   []byte("bad repo url"),
					},
				}).Build(),
			},
			args: args{
				ctx:      context.TODO(),
				autoheal: &autoheal,
				status:   status,
			},
			want: nil,
			err:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := AutohealConfigReconciler{
				Client: tt.fields.Client,
			}
			repoSecret, err := r.getConfigRepoSecret(tt.args.ctx, tt.args.autoheal, tt.args.status)
			if (err != nil) != tt.err {
				t.Errorf("GetConfigRepoSecret() error = %v wantedError= %v", err, tt.err)

			}
			if !reflect.DeepEqual(repoSecret, tt.want) {
				t.Errorf("GetConfigRepoSecret() got = %v, want %v", repoSecret, tt.want)
			}
		})
	}

}

func mockHttpClientGood(t *testing.T) *httptest.Server {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"tree":[{"path":"scenarios/fleetshard/example_broken_issue.yaml"},{"path":"scenarios/fleetshard/README.md"}]}`))
	}))

	return mockServer
}

func mockHttpClientBad(t *testing.T) *httptest.Server {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{[{}]}`))
	}))
	return mockServer
}

func mockHttpClientNoYaml(t *testing.T) *httptest.Server {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"tree":[{"path":"scenarios/fleetshard/README.md"},{"path":"scenarios/fleetshard/README2.md"},{"path":"scenarios/fleetshard"}]}`))
	}))

	return mockServer
}
func mockHttpClientGoodBad(t *testing.T) *httptest.Server {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Length", "1")
		w.Write([]byte(`{[{"path":""}]}`))
	}))

	return mockServer
}

func TestReadFiles(t *testing.T) {

	type fields struct {
		httpClient *httptest.Server
	}

	type args struct {
		repo *v1alpha1.ConfigRepositoryInfo
	}
	mockGood := mockHttpClientGood(t)
	mockBad := mockHttpClientBad(t)
	// mockNoYaml := mockHttpClientNoYaml(t)
	mockGoodBad := mockHttpClientGoodBad(t)

	tests := []struct {
		name   string
		fields fields
		args   args
		want   *v1alpha1.AutohealConfigTree
		err    bool
	}{
		{
			name: "Test data is present, correct and struct gets populated",
			fields: fields{
				mockGood,
			},
			args: args{
				repo: &v1alpha1.ConfigRepositoryInfo{Tag: tag, RepositoryURL: mockGood.URL, AccessToken: accessToken},
			},
			want: &configRepo,

			err: false,
		},

		{
			name: "Bad gateway, struct does not get populated",
			fields: fields{
				mockBad,
			},
			args: args{
				repo: &v1alpha1.ConfigRepositoryInfo{Tag: tag, RepositoryURL: mockBad.URL, AccessToken: accessToken},
			},
			want: nil,
			err:  false,
		},
		{
			name: "Cant make new request",
			fields: fields{
				mockBad,
			},
			args: args{
				repo: &v1alpha1.ConfigRepositoryInfo{Tag: tag, RepositoryURL: repoUrl, AccessToken: accessToken},
			},
			want: nil,
			err:  true,
		},
		// {
		// 	name: "No yaml files present",
		// 	fields: fields{
		// 		&mockNoYaml,
		// 	},
		// 	args: args{
		// 		repo: &v1alpha1.ConfigRepositoryInfo{Tag: tag, RepositoryURL: mockNoYaml.URL, AccessToken: accessToken},
		// 	},
		// 	want:  &v1alpha1.AutohealConfigTree{

		// 		Tree: []v1alpha1.AutohealConfigTreeResponse{},
		// 	},
		// 	err: false,
		// },
		{
			name: "Cant read request body",
			fields: fields{
				mockGoodBad,
			},
			args: args{
				repo: &v1alpha1.ConfigRepositoryInfo{Tag: tag, RepositoryURL: mockGoodBad.URL, AccessToken: accessToken},
			},
			want: nil,
			err:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := AutohealConfigReconciler{
				httpClient: *tt.fields.httpClient.Client(),
			}
			files, err := r.readFiles(tt.args.repo)
			if (err != nil) != tt.err {

				t.Errorf("readFiles() error = %v wantedError= %v", err, tt.err)

			}
			if !reflect.DeepEqual(files, tt.want) {
				fmt.Print("got", files)
				fmt.Print("want", tt.want)
				t.Errorf("readFiles() got = %v want %v", files, tt.want)
			}
		})
	}
}

func mockHttpClientGoodFiles(t *testing.T) *httptest.Server {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("name: Test-name\nquery: Test query >2\nlabel: test label\nscript: println(\"hello world\")\ngracePeriod: 2m"))
	}))

	return mockServer
}

func TestGetScenarios(t *testing.T) {
	mockGoodFiles := mockHttpClientGoodFiles(t)
	mockBad := mockHttpClientBad(t)
	mockGoodBad := mockHttpClientGoodBad(t)

	type fields struct {
		httpClient *httptest.Server
	}

	type args struct {
		repo     *v1alpha1.ConfigRepositoryInfo
		files    *v1alpha1.AutohealConfigTree
		autoheal *v1alpha1.AutohealConfig
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   []v1alpha1.AutohealConfigScenario
		err    bool
	}{
		{
			name: "Get files and populate struct",
			fields: fields{
				mockGoodFiles,
			},
			args: args{

				repo:     &v1alpha1.ConfigRepositoryInfo{Tag: tag, RepositoryURL: mockGoodFiles.URL, AccessToken: accessToken},
				files:    &configRepo,
				autoheal: &autoheal,
			},
			want: configScenario,
			err:  false,
		},

		{
			name: "Bad gateway, struct does not get populated",
			fields: fields{
				mockBad,
			},
			args: args{
				repo:     &v1alpha1.ConfigRepositoryInfo{Tag: tag, RepositoryURL: mockBad.URL, AccessToken: accessToken},
				files:    &configRepo,
				autoheal: &autoheal,
			},
			want: nil,
			err:  false,
		},
		{
			name: "Cant make new request",
			fields: fields{
				mockBad,
			},
			args: args{

				repo:     &v1alpha1.ConfigRepositoryInfo{Tag: tag, RepositoryURL: repoUrl, AccessToken: accessToken},
				files:    &configRepo,
				autoheal: &autoheal,
			},
			want: nil,
			err:  true,
		},
		// {
		// 	name: "No yaml files present",
		// 	fields: fields{
		// 		&mockNoYaml,
		// 	},
		// 	args: args{
		// 		repo: &v1alpha1.ConfigRepositoryInfo{Tag: tag, RepositoryURL: mockNoYaml.URL, AccessToken: accessToken},
		// 	},
		// 	want:  &v1alpha1.AutohealConfigTree{

		// 		Tree: []v1alpha1.AutohealConfigTreeResponse{},
		// 	},
		// 	err: false,
		// },
		{
			name: "Cant read request body",
			fields: fields{
				mockGoodBad,
			},
			args: args{

				repo:     &v1alpha1.ConfigRepositoryInfo{Tag: tag, RepositoryURL: mockGoodBad.URL, AccessToken: accessToken},
				files:    &configRepo,
				autoheal: &autoheal,
			},
			want: nil,
			err:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := AutohealConfigReconciler{
				httpClient: *tt.fields.httpClient.Client(),
			}
			files, err := r.getScenarios(tt.args.files, tt.args.repo, tt.args.autoheal)
			if (err != nil) != tt.err {

				t.Errorf("readFiles() error = %v wantedError= %v", err, tt.err)

			}
			if !reflect.DeepEqual(files, tt.want) {
				fmt.Print("got", files)
				fmt.Print("want", tt.want)
				t.Errorf("readFiles() got = %v want %v", files, tt.want)
			}
		})
	}
}

func TestConfigSecret(t *testing.T) {
	type fields struct {
		Client client.Client
	}

	type args struct {
		ctx      context.Context
		autoheal *v1alpha1.AutohealConfig
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		err    bool
	}{
		{
			name: "Test secret is deleted ",
			fields: fields{
				fake.NewClientBuilder().WithRuntimeObjects(&core.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: autoheal.Namespace, Name: autoheal.Spec.SecretName},
					Data: map[string][]byte{
						"tag":          []byte(tag),
						"access_token": []byte(accessToken),
						"repository":   []byte(repoUrl),
					},
				}).Build(),
			},
			args: args{
				ctx:      context.TODO(),
				autoheal: &autoheal,
			},
			err: false,
		},
		{
			name: "Test secret not found",
			fields: fields{
				fake.NewClientBuilder().WithRuntimeObjects(&core.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: autoheal.Namespace, Name: "test"},
					Data: map[string][]byte{
						"tag":          []byte(tag),
						"access_token": []byte(accessToken),
						"repository":   []byte(repoUrl),
					},
				}).Build(),
			},
			args: args{
				ctx:      context.TODO(),
				autoheal: &autoheal,
			},
			err: true,
		},
	
}

for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := AutohealConfigReconciler{
				Client: tt.fields.Client,
			}
			err := r.deleteConfigSecret( tt.args.autoheal,tt.args.ctx)
			if (err != nil) != tt.err {

				t.Errorf("readFiles() error = %v wantedError= %v", err, tt.err)

			}
		})
	}
}

// func TestConfigSecret(t *testing.T) {
// 	type fields struct {
// 		Client client.Client
// 	}

// 	type args struct {
// 		ctx      context.Context
// 		autoheal *v1alpha1.AutohealConfig
// 	}

// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 		err    bool
// 	}{
// 		{
// 			name: "Test secret is deleted ",
// 			fields: fields{
// 				fake.NewClientBuilder().WithRuntimeObjects(&core.ConfigMapList{
// 				}).Build(),
// 			},
// 			args: args{
// 				ctx:      context.TODO(),
// 				autoheal: &autoheal,
// 			},
// 			err: false,
// 		},
// 		{
// 			name: "Test secret not found",
// 			fields: fields{
// 				fake.NewClientBuilder().WithRuntimeObjects(&core.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: autoheal.Namespace, Name: "test"},
// 					Data: map[string][]byte{
// 						"tag":          []byte(tag),
// 						"access_token": []byte(accessToken),
// 						"repository":   []byte(repoUrl),
// 					},
// 				}).Build(),
// 			},
// 			args: args{
// 				ctx:      context.TODO(),
// 				autoheal: &autoheal,
// 			},
// 			err: true,
// 		},
	
// }

// for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			r := AutohealConfigReconciler{
// 				Client: tt.fields.Client,
// 			}
// 			err := r.deleteConfigSecret( tt.args.autoheal,tt.args.ctx)
// 			if (err != nil) != tt.err {

// 				t.Errorf("readFiles() error = %v wantedError= %v", err, tt.err)

// 			}
// 		})
// 	}
// }
