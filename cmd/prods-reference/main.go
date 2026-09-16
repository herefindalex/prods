package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"prods/internal/referencecatalog"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "create":
		err = runCreate(ctx, os.Args[2:])
	case "publish":
		err = runPublish(ctx, os.Args[2:])
	case "verify":
		err = runVerify(ctx, os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "prods-reference:", err)
		os.Exit(1)
	}
}

func runCreate(ctx context.Context, arguments []string) error {
	set := flag.NewFlagSet("create", flag.ContinueOnError)
	dataRoot := set.String("data-root", "", "new, dedicated reference data root (required)")
	seed := set.Int64("seed", referencecatalog.DefaultSeed, "deterministic fixture seed")
	if err := set.Parse(arguments); err != nil {
		return err
	}
	result, err := referencecatalog.Create(ctx, *dataRoot, *seed)
	if err != nil {
		return err
	}
	fmt.Printf("created %d hidden/archived reference products in %s\n", result.Products, result.DataRoot)
	fmt.Printf("manifest: %s\n", result.ManifestPath)
	fmt.Printf("fixture owner: %s\nfixture password: %s\n", result.OwnerEmail, result.Password)
	fmt.Println("no product is public yet; run the explicit publish command after inspecting the manifest")
	return nil
}

func runPublish(parent context.Context, arguments []string) error {
	set := flag.NewFlagSet("publish", flag.ContinueOnError)
	dataRoot := set.String("data-root", "", "existing reference data root (required)")
	baseURL := set.String("base-url", "https://catalog.example.test", "synthetic canonical base URL")
	timeout := set.Duration("timeout", 30*time.Minute, "maximum publication convergence time")
	if err := set.Parse(arguments); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, *timeout)
	defer cancel()
	result, err := referencecatalog.Publish(ctx, *dataRoot, *baseURL)
	if err != nil {
		return err
	}
	fmt.Printf("activated %d reference products\n", result.Published)
	fmt.Printf("public artifacts: %s\nmanifest: %s\n", result.PublicRoot, result.ManifestPath)
	return nil
}

func runVerify(ctx context.Context, arguments []string) error {
	set := flag.NewFlagSet("verify", flag.ContinueOnError)
	dataRoot := set.String("data-root", "", "existing reference data root (required)")
	if err := set.Parse(arguments); err != nil {
		return err
	}
	manifest, err := referencecatalog.Verify(ctx, *dataRoot)
	if err != nil {
		return err
	}
	fmt.Printf("verified reference dataset %s seed=%d products=%d publication_applied=%t\n", manifest.DatasetVersion, manifest.Seed, manifest.Summary.Products, manifest.PublicationApplied)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: prods-reference <create|publish|verify> --data-root PATH")
}
