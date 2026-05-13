package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	buildahDefine "go.podman.io/buildah/define"
	"go.podman.io/podman/v6/pkg/bindings"
	"go.podman.io/podman/v6/pkg/bindings/images"
	"go.podman.io/podman/v6/pkg/domain/entities/types"
)

const TargetTag = "repro-image:v1"

func podmanClient() (context.Context, error) {
	uri := fmt.Sprintf("unix:///run/user/%d/podman/podman.sock", os.Geteuid())
	c, err := bindings.NewConnection(context.Background(), uri)
	if err != nil {
		return nil, err
	}
	return c, nil

}

func main() {
	fmt.Println("=== Starting Podman Image Check Repro ===")

	// 1. Setup Connection
	uri := os.Getenv("DOCKER_HOST")
	if uri == "" {
		uri = fmt.Sprintf("unix:///run/user/%d/podman/podman.sock", os.Geteuid())
	}
	fmt.Printf("Using socket: %s\n", uri)

	ctx, err := podmanClient()
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to Podman: %v", err))
	}

	// 2. Run all 4 checks
	foundAPIExists := checkAPIExists(ctx)
	foundAPIList := checkAPIList(ctx)
	foundCLIExists := checkCLIExists()
	foundCLIImages := checkCLIImages()

	fmt.Println("\n--- Check Results ---")
	fmt.Printf("1. API images.Exists:      %v\n", foundAPIExists)
	fmt.Printf("2. API images.List:        %v\n", foundAPIList)
	fmt.Printf("3. CLI podman image exists: %v\n", foundCLIExists)
	fmt.Printf("4. CLI podman images:      %v\n", foundCLIImages)
	fmt.Println("---------------------")

	// 3. Decision to build
	anyFound := foundAPIExists || foundAPIList || foundCLIExists || foundCLIImages

	if anyFound {
		fmt.Println("\n✅ Image found by at least one method. Skipping build!")
	} else {
		fmt.Println("\n❌ Image NOT found. Starting build...")
		buildImage(ctx)
	}
}

// Method 1: Podman SDK images.Exists
func checkAPIExists(ctx context.Context) bool {
	exists, err := images.Exists(ctx, TargetTag, nil)
	if err != nil {
		fmt.Printf("[API Exists] Error: %v\n", err)
		return false
	}
	return exists
}

// Method 2: Podman SDK images.List
func checkAPIList(ctx context.Context) bool {
	a := true
	list, err := images.List(ctx, &images.ListOptions{All: &a})
	if err != nil {
		fmt.Printf("[API List] Error: %v\n", err)
		return false
	}

	for _, img := range list {
		for _, tag := range img.RepoTags {
			cleanTag := tag
			if idx := strings.LastIndex(tag, "/"); idx != -1 {
				cleanTag = tag[idx+1:]
			}

			fmt.Print("tag: %s\n", cleanTag)

			if cleanTag == TargetTag {
				return true
			}
		}
	}
	return false
}

// Method 3: CLI 'podman image exists'
func checkCLIExists() bool {
	cmd := exec.Command("podman", "image", "exists", TargetTag)
	err := cmd.Run()
	return err == nil
}

// Method 4: CLI 'podman images' text parsing
func checkCLIImages() bool {
	cmd := exec.Command("podman", "images", "--format", "{{.Repository}}:{{.Tag}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[CLI Images] Error: %v\n", err)
		return false
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == TargetTag {
			return true
		}
	}
	return false
}

// Build the image using the exact options from your project
func buildImage(ctx context.Context) {
	fmt.Println("Building alpine repro image...")

	buildOptions := types.BuildOptions{
		BuildOptions: buildahDefine.BuildOptions{
			ContextDirectory:        ".",
			Output:                  TargetTag,
			OutputFormat:            buildahDefine.OCIv1ImageManifest,
			RemoveIntermediateCtrs:  true,
			ForceRmIntermediateCtrs: true,
			Layers:                  true,
			Squash:                  true,
			Out:                     os.Stdout,
			Err:                     os.Stderr,
			ReportWriter:            os.Stdout,
		},
	}

	_, err := images.Build(ctx, []string{"Dockerfile"}, buildOptions)
	if err != nil {
		panic(fmt.Sprintf("Build failed: %v", err))
	}
	fmt.Println("Build completed successfully!")
}
