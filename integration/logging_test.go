package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paketo-buildpacks/occam"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
	. "github.com/paketo-buildpacks/occam/matchers"
)

func testLogging(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect

		pack   occam.Pack
		docker occam.Docker

		pullPolicy              = "never"
		extenderBuildStr        = ""
		extenderBuildStrEscaped = ""
	)

	it.Before(func() {
		pack = occam.NewPack()
		docker = occam.NewDocker()

		if settings.Extensions.UbiNodejsExtension.Online != "" {
			pullPolicy = "always"
			extenderBuildStr = "[extender (build)] "
			extenderBuildStrEscaped = `\[extender \(build\)\] `

		}
	})

	buildpackID := strings.ReplaceAll(buildpackInfo.Buildpack.ID, "/", "_")
	context("when app is NOT vendored", func() {
		var (
			image occam.Image

			name   string
			source string
		)

		it.Before(func() {
			var err error
			name, err = occam.RandomName()
			Expect(err).NotTo(HaveOccurred())
		})

		it.After(func() {
			Expect(docker.Image.Remove.Execute(image.ID)).To(Succeed())
			Expect(docker.Volume.Remove.Execute(occam.CacheVolumeNames(name))).To(Succeed())
			Expect(os.RemoveAll(source)).To(Succeed())
		})

		it("should build a working OCI image for a simple app", func() {
			var err error
			source, err = occam.Source(filepath.Join("testdata", "simple_app"))
			Expect(err).NotTo(HaveOccurred())

			var logs fmt.Stringer
			image, logs, err = pack.WithNoColor().Build.
				WithExtensions(
					settings.Extensions.UbiNodejsExtension.Online,
				).
				WithBuildpacks(
					nodeURI,
					pnpmURI,
					buildpackURI,
					buildPlanURI,
				).
				WithEnv(map[string]string{"BP_LOG_LEVEL": "DEBUG"}).
				WithPullPolicy(pullPolicy).
				Execute(name, source)
			Expect(err).NotTo(HaveOccurred())

			Expect(logs).To(ContainLines(
				fmt.Sprintf("%s%s %s", extenderBuildStr, buildpackInfo.Buildpack.Name, "1.2.3"),
				extenderBuildStr+"  Resolving installation process",
				extenderBuildStr+"    Process inputs:",
				extenderBuildStr+"      pnpm-lock.yaml -> Found",
				extenderBuildStr+"",
				extenderBuildStr+"    Selected default build process: 'pnpm install'",
				extenderBuildStr+"",
				extenderBuildStr+"  Executing launch environment install process",
				fmt.Sprintf(extenderBuildStr+"    Running 'pnpm install --frozen-lockfile --store-dir /layers/%s/pnpm-store'", buildpackID),
			))
			Expect(logs).To(ContainLines(
				extenderBuildStr+"  Configuring launch environment",
				extenderBuildStr+"    NODE_PROJECT_PATH -> \"/workspace\"",
				fmt.Sprintf("%s    PATH              -> \"$PATH:/layers/%s/launch-modules/node_modules/.bin\"", extenderBuildStr, buildpackID),
				extenderBuildStr+"",
				fmt.Sprintf(`%s  Generating SBOM for /layers/%s/launch-modules`, extenderBuildStr, buildpackID),
				MatchRegexp(extenderBuildStrEscaped+`      Completed in (\d+)(\.\d+)?(ms|s)`),
				extenderBuildStr+"",
				extenderBuildStr+"  Writing SBOM in the following format(s):",
				extenderBuildStr+"    application/vnd.cyclonedx+json",
				extenderBuildStr+"    application/spdx+json",
				extenderBuildStr+"    application/vnd.syft+json",
			))
		})
	})

	context("when the app is vendored", func() {

		//UBI does not support offline installation at the moment,
		//so we are skipping it.
		if settings.Extensions.UbiNodejsExtension.Online != "" {
			return
		}

		var (
			image occam.Image

			name   string
			source string
		)

		it.Before(func() {
			var err error
			name, err = occam.RandomName()
			Expect(err).NotTo(HaveOccurred())
		})

		it.After(func() {
			Expect(docker.Image.Remove.Execute(image.ID)).To(Succeed())
			Expect(docker.Volume.Remove.Execute(occam.CacheVolumeNames(name))).To(Succeed())
			Expect(os.RemoveAll(source)).To(Succeed())
		})

		it("should build a working OCI image for a simple app", func() {
			var err error
			source, err = occam.Source(filepath.Join("testdata", "vendored"))
			Expect(err).NotTo(HaveOccurred())

			var logs fmt.Stringer
			image, logs, err = pack.WithNoColor().Build.
				WithBuildpacks(
					nodeOfflineURI,
					pnpmOfflineURI,
					buildpackOfflineURI,
					buildPlanURI,
				).
				WithNetwork("none").
				WithPullPolicy("never").
				Execute(name, source)
			Expect(err).NotTo(HaveOccurred())

			buildpackID := buildpackID
			Expect(logs).To(ContainLines(
				fmt.Sprintf("%s %s", buildpackInfo.Buildpack.Name, "1.2.3"),
				"  Resolving installation process",
				"    Process inputs:",
				"      pnpm-lock.yaml -> Found",
				"",
				"    Selected default build process: 'pnpm install'",
				"",
				"  Executing launch environment install process",
				fmt.Sprintf("    Running 'pnpm install --frozen-lockfile --offline --store-dir /layers/%s/pnpm-store'", buildpackID),
			))
			Expect(logs).To(ContainLines(
				"  Configuring launch environment",
				"    NODE_PROJECT_PATH -> \"/workspace\"",
				fmt.Sprintf("    PATH              -> \"$PATH:/layers/%s/launch-modules/node_modules/.bin\"", buildpackID),
				"",
				fmt.Sprintf(`  Generating SBOM for /layers/%s/launch-modules`, buildpackID),
				MatchRegexp(`      Completed in (\d+)(\.\d+)?(ms|s)`),
				"",
			))
		})
	})

	context("when modules are required at build time", func() {
		var (
			image occam.Image

			name   string
			source string
		)

		it.Before(func() {
			var err error
			name, err = occam.RandomName()
			Expect(err).NotTo(HaveOccurred())
		})

		it.After(func() {
			Expect(docker.Image.Remove.Execute(image.ID)).To(Succeed())
			Expect(docker.Volume.Remove.Execute(occam.CacheVolumeNames(name))).To(Succeed())
			Expect(os.RemoveAll(source)).To(Succeed())
		})

		it("should build a working OCI image for a dev dependencies app", func() {
			var err error
			source, err = occam.Source(filepath.Join("testdata", "dev_dependencies_during_build"))
			Expect(err).NotTo(HaveOccurred())

			var logs fmt.Stringer
			image, logs, err = pack.WithNoColor().Build.
				WithExtensions(
					settings.Extensions.UbiNodejsExtension.Online,
				).
				WithBuildpacks(
					nodeURI,
					pnpmURI,
					buildpackURI,
					buildPlanURI,
				).
				WithEnv(map[string]string{"BP_LOG_LEVEL": "DEBUG"}).
				WithPullPolicy(pullPolicy).
				Execute(name, source)
			Expect(err).NotTo(HaveOccurred())

			buildpackID := buildpackID
			Expect(logs).To(ContainLines(
				fmt.Sprintf("%s%s %s", extenderBuildStr, buildpackInfo.Buildpack.Name, "1.2.3"),
				extenderBuildStr+"  Resolving installation process",
				extenderBuildStr+"    Process inputs:",
				extenderBuildStr+"      pnpm-lock.yaml -> Found",
				extenderBuildStr+"",
				extenderBuildStr+"    Selected default build process: 'pnpm install'",
				extenderBuildStr+"",
				extenderBuildStr+"  Executing build environment install process",
				fmt.Sprintf("%s    Running 'pnpm install --frozen-lockfile --prod false --store-dir /layers/%s/pnpm-store'", extenderBuildStr, buildpackID),
			))
			Expect(logs).To(ContainLines(
				extenderBuildStr+"  Configuring build environment",
				extenderBuildStr+`    NODE_ENV -> "development"`,
				fmt.Sprintf("%s    PATH     -> \"$PATH:/layers/%s/build-modules/node_modules/.bin\"", extenderBuildStr, buildpackID),
				extenderBuildStr+"",
				fmt.Sprintf(`%s  Generating SBOM for /layers/%s/build-modules`, extenderBuildStr, buildpackID),
				MatchRegexp(extenderBuildStrEscaped+`      Completed in (\d+)(\.\d+)?(ms|s)`),
				extenderBuildStr+"",
				extenderBuildStr+"  Writing SBOM in the following format(s):",
				extenderBuildStr+"    application/vnd.cyclonedx+json",
				extenderBuildStr+"    application/spdx+json",
				extenderBuildStr+"    application/vnd.syft+json",
				extenderBuildStr+"",
				extenderBuildStr+"  Resolving installation process",
				extenderBuildStr+"    Process inputs:",
				extenderBuildStr+"      pnpm-lock.yaml -> Found",
				extenderBuildStr+"",
				extenderBuildStr+"    Selected default build process: 'pnpm install'",
				extenderBuildStr+"",
				extenderBuildStr+"  Executing launch environment install process",
				fmt.Sprintf("%s    Running 'pnpm install --frozen-lockfile --store-dir /layers/%s/pnpm-store'", extenderBuildStr, buildpackID),
			))
			Expect(logs).To(ContainLines(
				extenderBuildStr+"  Configuring launch environment",
				extenderBuildStr+"    NODE_PROJECT_PATH -> \"/workspace\"",
				fmt.Sprintf("%s    PATH              -> \"$PATH:/layers/%s/launch-modules/node_modules/.bin\"", extenderBuildStr, buildpackID),
				extenderBuildStr+"",
				fmt.Sprintf(`%s  Generating SBOM for /layers/%s/launch-modules`, extenderBuildStr, buildpackID),
				MatchRegexp(extenderBuildStrEscaped+`      Completed in (\d+)(\.\d+)?(ms|s)`),
				extenderBuildStr+"",
				extenderBuildStr+"  Writing SBOM in the following format(s):",
				extenderBuildStr+"    application/vnd.cyclonedx+json",
				extenderBuildStr+"    application/spdx+json",
				extenderBuildStr+"    application/vnd.syft+json",
				extenderBuildStr+"",
			))
		})
	})
}
