// Copyright 2025 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"strings"

	"github.com/openfga/go-sdk/client"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/canonical/tenant-service/internal/authorization"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/monitoring"
	"github.com/canonical/tenant-service/internal/openfga"
	"github.com/canonical/tenant-service/internal/tracing"
)

const StoreName = "tenant-service"

// createFgaModelCmd represents the createFgaModel command
var createFgaModelCmd = &cobra.Command{
	Use:   "create-fga-model",
	Short: "Creates an openfga model",
	Long:  `Creates an openfga model`,
	Run: func(cmd *cobra.Command, args []string) {
		apiUrl, _ := cmd.Flags().GetString("fga-api-url")
		apiToken, _ := cmd.Flags().GetString("fga-api-token")
		storeId, _ := cmd.Flags().GetString("fga-store-id")
		format, _ := cmd.Flags().GetString("format")
		verbose, _ := cmd.Flags().GetBool("verbose")
		configMapResource, _ := cmd.Flags().GetString("store-k8s-configmap-resource")
		kubeconfigPath, _ := cmd.Flags().GetString("kubeconfig")

		modelId, finalStoreId, err := createModel(apiUrl, apiToken, storeId, verbose)
		if err != nil {
			cmd.PrintErrln(err)
			os.Exit(1)
		}

		if configMapResource != "" {
			if err := updateConfigMap(cmd.Context(), kubeconfigPath, configMapResource, finalStoreId, modelId); err != nil {
				cmd.PrintErrln(fmt.Errorf("failed to update configmap: %w", err))
				os.Exit(1)
			}
			cmd.Printf("ConfigMap %s updated successfully\n", configMapResource)
		}

		if format == "json" {
			output := struct {
				StoreId string `json:"store_id"`
				ModelId string `json:"model_id"`
			}{
				StoreId: finalStoreId,
				ModelId: modelId,
			}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(output); err != nil {
				cmd.PrintErrln(fmt.Errorf("failed to encode output: %v", err))
				os.Exit(1)
			}
		} else {
			cmd.Printf("Created model: %s\n", modelId)
			if storeId == "" {
				cmd.Printf("Created store: %s\n", finalStoreId)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(createFgaModelCmd)

	createFgaModelCmd.Flags().String("fga-api-url", "", "The openfga API URL")
	createFgaModelCmd.Flags().String("fga-api-token", "", "The openfga API token")
	createFgaModelCmd.Flags().String("fga-store-id", "", "The openfga store to create the model in, if empty one will be created")
	createFgaModelCmd.Flags().String("format", "text", "Output format (text or json)")
	createFgaModelCmd.Flags().BoolP("verbose", "v", false, "Enable verbose logging")
	createFgaModelCmd.Flags().String("store-k8s-configmap-resource", "", "The configmap resource to store the FGA Store ID and Model ID, format: namespace/name")
	createFgaModelCmd.Flags().String("kubeconfig", "", "Path to the kubeconfig file (optional, defaults to in-cluster config)")
	createFgaModelCmd.MarkFlagRequired("fga-api-url")
	createFgaModelCmd.MarkFlagRequired("fga-api-token")
}

func createModel(apiUrl, apiToken, storeId string, verbose bool) (string, string, error) {
	ctx := context.Background()

	logger := logging.NewNoopLogger()
	tracer := tracing.NewNoopTracer()
	monitor := monitoring.NewNoopMonitor("", logger)

	scheme, host, err := parseURL(apiUrl)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse url: %w", err)
	}

	// skip validation for openfga object
	cfg := openfga.Config{
		ApiScheme:   scheme,
		ApiHost:     host,
		StoreID:     storeId,
		ApiToken:    apiToken,
		AuthModelID: "",
		Debug:       verbose,
		Tracer:      tracer,
		Monitor:     monitor,
		Logger:      logger,
	}

	fgaClient := openfga.NewClient(&cfg)

	if storeId == "" {
		storeId, err = fgaClient.CreateStore(ctx, StoreName)

		if err != nil {
			return "", "", fmt.Errorf("failed to create store: %w", err)
		}

		fgaClient.SetStoreID(ctx, storeId)
	}

	authzModel := authorization.NewAuthorizationModelProvider("v0").
		GetModel()

	modelId, err := fgaClient.WriteModel(
		context.Background(),
		&client.ClientWriteAuthorizationModelRequest{
			TypeDefinitions: authzModel.TypeDefinitions,
			SchemaVersion:   authzModel.SchemaVersion,
			Conditions:      authzModel.Conditions,
		},
	)

	if err != nil {
		return "", "", fmt.Errorf("failed to write model: %w", err)
	}

	return modelId, storeId, nil
}

func parseURL(s string) (string, string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", "", err
	}
	return u.Scheme, u.Host, nil
}

func updateConfigMap(ctx context.Context, kubeconfigPath, configMapResource, storeId, modelId string) error {
	parts := strings.Split(configMapResource, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid configmap resource format: %s, expected namespace/name", configMapResource)
	}
	namespace, name := parts[0], parts[1]

	var config *rest.Config
	var err error

	if kubeconfigPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			// Fallback to kubeconfig if in-cluster fails (e.g. running locally without flag)
			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			configOverrides := &clientcmd.ConfigOverrides{}
			kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
			config, err = kubeConfig.ClientConfig()
		}
	}
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Try to Create it
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Data: map[string]string{
					"OPENFGA_STORE_ID":               storeId,
					"OPENFGA_AUTHORIZATION_MODEL_ID": modelId,
				},
			}
			_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create configmap %s: %w", configMapResource, err)
			}
			return nil
		}
		return fmt.Errorf("failed to get configmap %s: %w", configMapResource, err)
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}

	cm.Data["OPENFGA_STORE_ID"] = storeId
	cm.Data["OPENFGA_AUTHORIZATION_MODEL_ID"] = modelId

	_, err = clientset.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update configmap %s: %w", configMapResource, err)
	}

	return nil
}
