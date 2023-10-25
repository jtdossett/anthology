package registry

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/erikvanbrakel/anthology/app"
	"github.com/erikvanbrakel/anthology/models"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

type OrasRegistry struct {
	registryUrl string
}

func (r *OrasRegistry) ListModules(namespace, name, provider string, offset, limit int) (modules []models.Module, total int, err error) {
	modules, err = r.getModules(namespace, name, provider)

	if err != nil {
		return nil, 0, err
	}

	return modules, len(modules), nil
}

func (r *OrasRegistry) PublishModule(namespace, name, provider, version string, data io.Reader) (err error) {

	ctx := context.Background()

	reg := r.GetRegistry()

	reponame := namespace + "/" + name

	repo, err := reg.Repository(ctx, reponame)
	handleError(err)

	fs, err := file.New("/tmp/")
	handleError(err)
	defer fs.Close()

	buffer, err := io.ReadAll(data)

	os.WriteFile("/tmp/module.tgz", buffer, 0644)

	mediaType := "application/vnd.opentofu.module.v1+tgz"
	fileNames := []string{"/tmp/module.tgz"}

	fileDescriptors := make([]v1.Descriptor, 0, len(fileNames))
	for _, name := range fileNames {
		fileDescriptor, err := fs.Add(ctx, name, mediaType, "")
		if err != nil {
			panic(err)
		}
		fileDescriptors = append(fileDescriptors, fileDescriptor)
	}

	artifactType := "application/vnd.opentofu.module"
	opts := oras.PackManifestOptions{
		Layers: fileDescriptors,
		ManifestAnnotations: map[string]string{
			"org.opentofu.module.provider":  provider,
			"org.opentofu.module.version":   version,
			"org.opentofu.module.namespace": namespace,
		},
	}
	manifestDescriptor, err := oras.PackManifest(ctx, fs, oras.PackManifestVersion1_1_RC4, artifactType, opts)
	if err != nil {
		panic(err)
	}

	tag := version
	if err = fs.Tag(ctx, manifestDescriptor, tag); err != nil {
		panic(err)
	}

	_, err = oras.Copy(ctx, fs, tag, repo, tag, oras.DefaultCopyOptions)

	return err
}

func (r *OrasRegistry) GetModuleData(namespace, name, provider, version string) (reader *bytes.Buffer, err error) {
	panic("implement me")
}

func (r *OrasRegistry) getModules(namespace, name, provider string) (modules []models.Module, err error) {
	ctx := context.Background()

	reg := r.GetRegistry()

	reponame := namespace + "/" + name
	fmt.Println("Repo name = " + reponame)

	handleError(err)

	err = reg.Repositories(ctx, "", func(repos []string) error {
		for _, repo := range repos {

			repoex, err := reg.Repository(ctx, repo)
			handleError(err)

			repoex.Tags(ctx, "", func(tags []string) error {
				for _, tag := range tags {
					modules = append(modules, models.Module{
						Name:      strings.Split(repo, "/")[1],
						Namespace: strings.Split(repo, "/")[0],
						Provider:  provider,
						Version:   tag,
					})
				}
				return nil
			})

		}
		return nil
	})

	return modules, err
}

func handleError(err error) {
	if err != nil {
		fmt.Println(err)
		panic("Error caught")
	}
}

func (r *OrasRegistry) GetRegistry() *remote.Registry {

	var err error

	username := os.Getenv("REGISTRY_USERNAME")
	password := os.Getenv("REGISTRY_PASSWORD")

	creds := auth.Credential{Username: username, Password: password}

	reg, err := remote.NewRegistry(r.registryUrl)
	handleError(err)

	reg.RepositoryOptions.Client = &auth.Client{
		Credential: auth.StaticCredential(r.registryUrl, creds),
	}

	reg.Client = &auth.Client{
		Credential: auth.StaticCredential(r.registryUrl, creds),
	}

	fmt.Println("Pinging with creds ...")
	err = reg.Ping(context.Background())
	handleError(err)

	fmt.Println("Pinging successful ...")

	return reg
}

func NewOrasRegistry(options app.OrasOptions) Registry {
	return &OrasRegistry{
		options.RegistryUrl,
	}
}
