// Command box-sdk-example is a small usage example for the Go Box SDK. It
// authenticates with the Client Credentials Grant, lists the items in the root
// folder, and prints them. Credentials are read from the environment so the
// example can be compiled without secrets baked in.
//
// Required environment variables:
//
//	BOX_CLIENT_ID, BOX_CLIENT_SECRET, BOX_ENTERPRISE_ID
//
// Run with: go run ./cmd
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/asalih/go-box-sdk/box"
	"github.com/asalih/go-box-sdk/managers"
)

func main() {
	clientID := os.Getenv("BOX_CLIENT_ID")
	clientSecret := os.Getenv("BOX_CLIENT_SECRET")
	enterpriseID := os.Getenv("BOX_ENTERPRISE_ID")
	if clientID == "" || clientSecret == "" || enterpriseID == "" {
		log.Fatal("set BOX_CLIENT_ID, BOX_CLIENT_SECRET, and BOX_ENTERPRISE_ID")
	}

	auth := box.NewBoxCcgAuth(box.CcgConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		EnterpriseID: enterpriseID,
	})
	client := box.NewBoxClient(auth, nil)

	ctx := context.Background()
	items, err := client.Folders.GetFolderItems(ctx, "0", &managers.GetFolderItemsOptions{Limit: "100"})
	if err != nil {
		log.Fatalf("list root folder: %v", err)
	}

	for _, item := range items.Entries {
		fmt.Printf("%s\t%s\t%s\n", item.Type, item.ID, item.Name)
	}
}
