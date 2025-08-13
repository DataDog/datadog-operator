package main

import (
	"fmt"
	"log"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

var (
	registry = "gcr.io/datadog-public/datadog"
	imageTag = os.Getenv("FULL_TAG")
	imageRef = fmt.Sprintf("%s/datadog-operator:%s", registry, imageTag)
)

func main() {
	log.Printf("Annotating index manifest %s...\n", imageRef)
	// Parse the image reference
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		log.Fatalf("failed to parse reference: %v", err)
	}

	// Pull the existing image index
	index, err := remote.Index(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		log.Fatalf("failed to pull index: %v", err)
	}

	// Add service name annotation
	annotated := mutate.Annotations(index, map[string]string{
		"com.googleapis.cloudmarketplace.product.service.name": "services/datadog-datadog-saas.cloudpartnerservices.goog",
	})

	// Convert to v1.ImageIndex for pushing
	idx, ok := annotated.(v1.ImageIndex)
	if !ok {
		log.Fatal("annotated object is not a v1.ImageIndex")
	}

	// Push the annotated index
	if err := remote.WriteIndex(ref, idx, remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		log.Fatalf("failed to push annotated index: %v", err)
	}

	log.Printf("✅ Successfully pushed annotated index to: %s\n", ref.Name())
}
