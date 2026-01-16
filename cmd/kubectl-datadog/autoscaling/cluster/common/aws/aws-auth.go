package aws

import (
	"context"
	"fmt"
	"log"
	"slices"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type RoleMapping struct {
	RoleArn  string   `yaml:"rolearn"`
	Username string   `yaml:"username"`
	Groups   []string `yaml:"groups"`
}

func EnsureAwsAuthRole(ctx context.Context, clientset kubernetes.Interface, roleMapping RoleMapping) error {
	cm, err := clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "aws-auth", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get aws-auth ConfigMap: %w", err)
	}

	var roles []RoleMapping
	if mapRoles, ok := cm.Data["mapRoles"]; ok {
		if err = yaml.Unmarshal([]byte(mapRoles), &roles); err != nil {
			return fmt.Errorf("failed to parse mapRoles: %w", err)
		}
	} else {
		roles = make([]RoleMapping, 0, 1)
	}

	for _, role := range roles {
		if role.RoleArn == roleMapping.RoleArn {
			log.Printf("Role %s already exists in aws-auth ConfigMap.", roleMapping.RoleArn)
			return nil
		}
	}

	roles = append(roles, roleMapping)

	updated, err := yaml.Marshal(roles)
	if err != nil {
		return fmt.Errorf("failed to marshal updated mapRoles: %w", err)
	}

	cm.Data["mapRoles"] = string(updated)

	if _, err := clientset.CoreV1().ConfigMaps("kube-system").Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update aws-auth ConfigMap: %w", err)
	}

	log.Printf("Added role %s to aws-auth ConfigMap.", roleMapping.RoleArn)

	return nil
}

func RemoveAwsAuthRole(ctx context.Context, clientset kubernetes.Interface, roleArn string) error {
	cm, err := clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "aws-auth", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get aws-auth ConfigMap: %w", err)
	}

	var roles []RoleMapping
	if mapRoles, ok := cm.Data["mapRoles"]; ok {
		if err = yaml.Unmarshal([]byte(mapRoles), &roles); err != nil {
			return fmt.Errorf("failed to parse mapRoles: %w", err)
		}
	} else {
		log.Printf("No mapRoles found in aws-auth ConfigMap, skipping role removal.")
		return nil
	}

	oldLen := len(roles)
	roles = slices.DeleteFunc(roles, func(role RoleMapping) bool {
		return role.RoleArn == roleArn
	})

	if len(roles) == oldLen {
		log.Printf("Role %s not found in aws-auth ConfigMap, skipping removal.", roleArn)
		return nil
	}

	updated, err := yaml.Marshal(roles)
	if err != nil {
		return fmt.Errorf("failed to marshal updated mapRoles: %w", err)
	}

	cm.Data["mapRoles"] = string(updated)

	if _, err := clientset.CoreV1().ConfigMaps("kube-system").Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update aws-auth ConfigMap: %w", err)
	}

	log.Printf("Removed role %s from aws-auth ConfigMap.", roleArn)

	return nil
}
