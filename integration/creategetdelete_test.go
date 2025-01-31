// +build integration

package integration_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	awseks "github.com/aws/aws-sdk-go/service/eks"
	harness "github.com/dlespiau/kube-test-harness"
	"github.com/kubicorn/kubicorn/pkg/namer"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	. "github.com/weaveworks/eksctl/integration/matchers"
	. "github.com/weaveworks/eksctl/integration/runner"

	api "github.com/weaveworks/eksctl/pkg/apis/eksctl.io/v1alpha5"
	"github.com/weaveworks/eksctl/pkg/authconfigmap"
	"github.com/weaveworks/eksctl/pkg/ctl/cmdutils"
	"github.com/weaveworks/eksctl/pkg/eks"
	"github.com/weaveworks/eksctl/pkg/iam"
	"github.com/weaveworks/eksctl/pkg/utils/file"
)

var _ = Describe("(Integration) Create, Get, Scale & Delete", func() {

	const (
		initNG = "ng-0"
		testNG = "ng-1"
	)

	commonTimeout := 10 * time.Minute

	BeforeSuite(func() {
		kubeconfigTemp = false
		if kubeconfigPath == "" {
			wd, _ := os.Getwd()
			f, _ := ioutil.TempFile(wd, "kubeconfig-")
			kubeconfigPath = f.Name()
			kubeconfigTemp = true
		}
	})

	AfterSuite(func() {
		gexec.KillAndWait()
		if kubeconfigTemp {
			os.Remove(kubeconfigPath)
		}
		os.RemoveAll(testDirectory)
	})

	Describe("when creating a cluster with 1 node", func() {
		It("should not return an error", func() {
			if !doCreate {
				fmt.Fprintf(GinkgoWriter, "will use existing cluster %s", clusterName)
				if !file.Exists(kubeconfigPath) {
					// Generate the Kubernetes configuration that eksctl create
					// would have generated otherwise:
					cmd := eksctlUtilsCmd.WithArgs(
						"write-kubeconfig",
						"--verbose", "4",
						"--name", clusterName,
						"--kubeconfig", kubeconfigPath,
					)
					Expect(cmd).To(RunSuccessfully())
				}
				return
			}

			fmt.Fprintf(GinkgoWriter, "Using kubeconfig: %s\n", kubeconfigPath)

			if clusterName == "" {
				clusterName = cmdutils.ClusterName("", "")
			}

			cmd := eksctlCreateCmd.WithArgs(
				"cluster",
				"--verbose", "4",
				"--name", clusterName,
				"--tags", "alpha.eksctl.io/description=eksctl integration test",
				"--nodegroup-name", initNG,
				"--node-labels", "ng-name="+initNG,
				"--node-type", "t2.medium",
				"--nodes", "1",
				"--version", version,
				"--kubeconfig", kubeconfigPath,
			)
			Expect(cmd).To(RunSuccessfully())
		})

		It("should have created an EKS cluster and two CloudFormation stacks", func() {
			awsSession := NewSession(region)

			Expect(awsSession).To(HaveExistingCluster(clusterName, awseks.ClusterStatusActive, version))

			Expect(awsSession).To(HaveExistingStack(fmt.Sprintf("eksctl-%s-cluster", clusterName)))
			Expect(awsSession).To(HaveExistingStack(fmt.Sprintf("eksctl-%s-nodegroup-%s", clusterName, initNG)))
		})

		It("should have created a valid kubectl config file", func() {
			config, err := clientcmd.LoadFromFile(kubeconfigPath)
			Expect(err).ShouldNot(HaveOccurred())

			err = clientcmd.ConfirmUsable(*config, "")
			Expect(err).ShouldNot(HaveOccurred())

			Expect(config.CurrentContext).To(ContainSubstring("eksctl"))
			Expect(config.CurrentContext).To(ContainSubstring(clusterName))
			Expect(config.CurrentContext).To(ContainSubstring(region))
		})

		Context("and listing clusters", func() {
			It("should return the previously created cluster", func() {
				cmd := eksctlGetCmd.WithArgs("clusters", "--all-regions")
				Expect(cmd).To(RunSuccessfullyWithOutputString(ContainSubstring(clusterName)))
			})
		})

		Context("and configuring Flux for GitOps", func() {
			It("should not return an error", func() {
				// Use a random branch to ensure test runs don't step on each others.
				branch := namer.RandomName()
				cloneDir, err := createBranch(branch)
				Expect(err).ShouldNot(HaveOccurred())
				defer deleteBranch(branch, cloneDir)

				assertFluxManifestsAbsentInGit(branch)
				assertFluxPodsAbsentInKubernetes(kubeconfigPath)

				cmd := eksctlExperimentalCmd.WithArgs(
					"install", "flux",
					"--git-url", Repository,
					"--git-email", Email,
					"--git-private-ssh-key-path", privateSSHKeyPath,
					"--git-branch", branch,
					"--name", clusterName,
				)
				Expect(cmd).To(RunSuccessfully())

				assertFluxManifestsPresentInGit(branch)
				assertFluxPodsPresentInKubernetes(kubeconfigPath)
			})
		})

		Context("gitops apply", func() {
			It("should add quickstart to the repo and the cluster", func() {
				// Use a random branch to ensure test runs don't step on each others.
				branch := namer.RandomName()
				cloneDir, err := createBranch(branch)
				Expect(err).ShouldNot(HaveOccurred())
				defer deleteBranch(branch, cloneDir)

				tempOutputDir, err := ioutil.TempDir(os.TempDir(), "gitops-repo-")
				assertFluxManifestsAbsentInGit(branch)

				deleteFluxInstallation(kubeconfigPath)
				assertFluxPodsAbsentInKubernetes(kubeconfigPath)

				cmd := eksctlExperimentalCmd.WithArgs(
					"gitops", "apply",
						"--git-url", Repository,
						"--git-email", Email,
						"--git-branch", branch,
						"--git-private-ssh-key-path", privateSSHKeyPath,
						"--output-path", tempOutputDir,
						"--quickstart-profile", "app-dev",
						"--cluster", clusterName,
						"--region", region,
				)
				Expect(cmd).To(RunSuccessfully())

				assertQuickStartComponentsPresentInGit(branch)
				assertFluxManifestsPresentInGit(branch)
				assertFluxPodsPresentInKubernetes(kubeconfigPath)
			})
		})

		Context("and scale the initial nodegroup", func() {
			It("should not return an error", func() {
				cmd := eksctlScaleNodeGroupCmd.WithArgs(
					"--cluster", clusterName,
					"--nodes", "4",
					"--name", initNG,
				)
				Expect(cmd).To(RunSuccessfully())
			})

			It("{FLAKY: https://github.com/weaveworks/eksctl/issues/717} should make it 4 nodes total", func() {
				test, err := newKubeTest()
				Expect(err).ShouldNot(HaveOccurred())
				defer test.Close()

				test.WaitForNodesReady(4, commonTimeout)

				nodes := test.ListNodes((metav1.ListOptions{
					LabelSelector: api.NodeGroupNameLabel + "=" + initNG,
				}))

				Expect(len(nodes.Items)).To(Equal(4))
			})
		})

		Context("and add the second nodegroup", func() {
			It("should not return an error", func() {
				cmd := eksctlCreateCmd.WithArgs(
					"nodegroup",
					"--cluster", clusterName,
					"--nodes", "4",
					"--node-private-networking",
					testNG,
				)
				Expect(cmd).To(RunSuccessfully())
			})

			It("should be able to list nodegroups", func() {
				{
					cmd := eksctlGetCmd.WithArgs(
						"nodegroup",
						"--cluster", clusterName,
						"--region", region,
						initNG,
					)
					Expect(cmd).To(RunSuccessfullyWithOutputStringLines(
						ContainElement(ContainSubstring(initNG)),
						Not(ContainElement(ContainSubstring(testNG))),
					))
				}
				{
					cmd := eksctlGetCmd.WithArgs(
						 "nodegroup",
						"--cluster", clusterName,
						"--region", region,
						testNG,
					)
					Expect(cmd).To(RunSuccessfullyWithOutputStringLines(
						ContainElement(ContainSubstring(testNG)),
						Not(ContainElement(ContainSubstring(initNG))),
					))
				}
				{
					cmd := eksctlGetCmd.WithArgs(
						"nodegroup",
						"--cluster", clusterName,
						"--region", region,
					)
					Expect(cmd).To(RunSuccessfullyWithOutputStringLines(
						ContainElement(ContainSubstring(testNG)),
						ContainElement(ContainSubstring(initNG)),
					))
				}
			})

			It("{FLAKY: https://github.com/weaveworks/eksctl/issues/717} should make it 8 nodes total", func() {
				test, err := newKubeTest()
				Expect(err).ShouldNot(HaveOccurred())
				defer test.Close()

				test.WaitForNodesReady(8, commonTimeout)

				nodes := test.ListNodes(metav1.ListOptions{})

				Expect(len(nodes.Items)).To(Equal(8))
			})

			Context("create test workloads", func() {
				var (
					err  error
					test *harness.Test
				)

				BeforeEach(func() {
					test, err = newKubeTest()
					Expect(err).ShouldNot(HaveOccurred())
				})

				AfterEach(func() {
					test.Close()
					Eventually(func() int {
						return len(test.ListPods(test.Namespace, metav1.ListOptions{}).Items)
					}, "3m", "1s").Should(BeZero())
				})

				It("should deploy podinfo service to the cluster and access it via proxy", func() {
					d := test.CreateDeploymentFromFile(test.Namespace, "podinfo.yaml")
					test.WaitForDeploymentReady(d, commonTimeout)

					pods := test.ListPodsFromDeployment(d)
					Expect(len(pods.Items)).To(Equal(2))

					// For each pod of the Deployment, check we receive a sensible response to a
					// GET request on /version.
					for _, pod := range pods.Items {
						Expect(pod.Namespace).To(Equal(test.Namespace))

						req := test.PodProxyGet(&pod, "", "/version")
						fmt.Fprintf(GinkgoWriter, "url = %#v", req.URL())

						var js interface{}
						test.PodProxyGetJSON(&pod, "", "/version", &js)

						Expect(js.(map[string]interface{})).To(HaveKeyWithValue("version", "1.5.1"))
					}
				})

				It("should have functional DNS", func() {
					d := test.CreateDaemonSetFromFile(test.Namespace, "test-dns.yaml")

					test.WaitForDaemonSetReady(d, commonTimeout)

					{
						ds, err := test.GetDaemonSet(test.Namespace, d.Name)
						Expect(err).ShouldNot(HaveOccurred())
						fmt.Fprintf(GinkgoWriter, "ds.Status = %#v", ds.Status)
					}
				})

				It("should have access to HTTP(S) sites", func() {
					d := test.CreateDaemonSetFromFile(test.Namespace, "test-http.yaml")

					test.WaitForDaemonSetReady(d, commonTimeout)

					{
						ds, err := test.GetDaemonSet(test.Namespace, d.Name)
						Expect(err).ShouldNot(HaveOccurred())
						fmt.Fprintf(GinkgoWriter, "ds.Status = %#v", ds.Status)
					}
				})
			})

			Context("toggle CloudWatch logging", func() {
				var (
					cfg *api.ClusterConfig
					ctl *eks.ClusterProvider
				)

				BeforeEach(func() {
					cfg = &api.ClusterConfig{
						Metadata: &api.ClusterMeta{
							Name:   clusterName,
							Region: region,
						},
					}
					ctl = eks.New(&api.ProviderConfig{Region: region}, cfg)
				})

				It("should have all types disabled by default", func() {
					enabled, disable, err := ctl.GetCurrentClusterConfigForLogging(cfg.Metadata)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(enabled.List()).To(HaveLen(0))
					Expect(disable.List()).To(HaveLen(5))
				})

				It("should plan to enable two of the types using flags", func() {
					cmd := eksctlUtilsCmd.WithArgs(
						"update-cluster-logging",
						"--name", clusterName,
						"--enable-types", "api,controllerManager",
					)
					Expect(cmd).To(RunSuccessfully())

					enabled, disable, err := ctl.GetCurrentClusterConfigForLogging(cfg.Metadata)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(enabled.List()).To(HaveLen(0))
					Expect(disable.List()).To(HaveLen(5))
				})

				It("should enable two of the types using flags", func() {
					cmd := eksctlUtilsCmd.WithArgs(
						"update-cluster-logging",
						"--name", clusterName,
						"--approve",
						"--enable-types", "api,controllerManager",
					)
					Expect(cmd).To(RunSuccessfully())

					enabled, disable, err := ctl.GetCurrentClusterConfigForLogging(cfg.Metadata)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(enabled.List()).To(HaveLen(2))
					Expect(disable.List()).To(HaveLen(3))
					Expect(enabled.List()).To(ConsistOf("api", "controllerManager"))
					Expect(disable.List()).To(ConsistOf("audit", "authenticator", "scheduler"))
				})

				It("should enable all of the types with --enable-types=all", func() {
					cmd := eksctlUtilsCmd.WithArgs(
						"update-cluster-logging",
						"--name", clusterName,
						"--approve",
						"--enable-types", "all",
					)
					Expect(cmd).To(RunSuccessfully())

					enabled, disable, err := ctl.GetCurrentClusterConfigForLogging(cfg.Metadata)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(enabled.List()).To(HaveLen(5))
					Expect(disable.List()).To(HaveLen(0))
				})

				It("should enable all but one type", func() {
					cmd := eksctlUtilsCmd.WithArgs(
						"update-cluster-logging",
						"--name", clusterName,
						"--approve",
						"--enable-types", "all",
						"--disable-types", "controllerManager",
					)
					Expect(cmd).To(RunSuccessfully())

					enabled, disable, err := ctl.GetCurrentClusterConfigForLogging(cfg.Metadata)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(enabled.List()).To(HaveLen(4))
					Expect(disable.List()).To(HaveLen(1))
					Expect(enabled.List()).To(ConsistOf("api", "audit", "authenticator", "scheduler"))
					Expect(disable.List()).To(ConsistOf("controllerManager"))
				})

				It("should disable all but one type", func() {
					cmd := eksctlUtilsCmd.WithArgs(
						"update-cluster-logging",
						"--name", clusterName,
						"--approve",
						"--disable-types", "all",
						"--enable-types", "controllerManager",
					)
					Expect(cmd).To(RunSuccessfully())

					enabled, disable, err := ctl.GetCurrentClusterConfigForLogging(cfg.Metadata)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(disable.List()).To(HaveLen(4))
					Expect(enabled.List()).To(HaveLen(1))
					Expect(disable.List()).To(ConsistOf("api", "audit", "authenticator", "scheduler"))
					Expect(enabled.List()).To(ConsistOf("controllerManager"))
				})

				It("should disable all of the types with --disable-types=all", func() {
					cmd := eksctlUtilsCmd.WithArgs(
						"update-cluster-logging",
						"--name", clusterName,
						"--approve",
						"--disable-types", "all",
					)
					Expect(cmd).To(RunSuccessfully())

					enabled, disable, err := ctl.GetCurrentClusterConfigForLogging(cfg.Metadata)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(enabled.List()).To(HaveLen(0))
					Expect(disable.List()).To(HaveLen(5))
					Expect(disable.HasAll(api.SupportedCloudWatchClusterLogTypes()...)).To(BeTrue())
				})
			})

			Context("and manipulating iam identity mappings", func() {
				var (
					role, exp0, exp1 string
					role0, role1     authconfigmap.MapRole
				)

				BeforeEach(func() {
					role = "arn:aws:iam::123456:role/eksctl-testing-XYZ"

					role0 = authconfigmap.MapRole{
						RoleARN: role,
						Identity: iam.Identity{
							Username: "admin",
							Groups:   []string{"system:masters", "system:nodes"},
						},
					}
					role1 = authconfigmap.MapRole{
						RoleARN: role,
						Identity: iam.Identity{
							Groups: []string{"system:something"},
						},
					}

					bs, err := yaml.Marshal([]authconfigmap.MapRole{role0})
					Expect(err).ShouldNot(HaveOccurred())
					exp0 = string(bs)

					bs, err = yaml.Marshal([]authconfigmap.MapRole{role1})
					Expect(err).ShouldNot(HaveOccurred())
					exp1 = string(bs)
				})

				It("fails getting unknown mapping", func() {
					cmd := eksctlGetCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", "idontexist",
						"-o", "yaml",
					)
					Expect(cmd).ToNot(RunSuccessfully())
				})
				It("creates mapping", func() {
					createCmd := eksctlCreateCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", role0.RoleARN,
						"--username", role0.Username,
						"--group", role0.Groups[0],
						"--group", role0.Groups[1],
					)
					Expect(createCmd).To(RunSuccessfully())

					getCmd := eksctlGetCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", role0.RoleARN,
						"-o", "yaml",
					)
					Expect(getCmd).To(RunSuccessfullyWithOutputString(MatchYAML(exp0)))
				})
				It("creates a duplicate mapping", func() {
					createCmd := eksctlCreateCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", role0.RoleARN,
						"--username", role0.Username,
						"--group", role0.Groups[0],
						"--group", role0.Groups[1],
					)
					Expect(createCmd).To(RunSuccessfully())

					getCmd := eksctlGetCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", role0.RoleARN,
						"-o", "yaml",
					)
					Expect(getCmd).To(RunSuccessfullyWithOutputString(MatchYAML(exp0 + exp0)))
				})
				It("creates a duplicate mapping with different identity", func() {
					createCmd := eksctlCreateCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", role1.RoleARN,
						"--group", role1.Groups[0],
					)
					Expect(createCmd).To(RunSuccessfully())

					getCmd := eksctlGetCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", role1.RoleARN,
						"-o", "yaml",
					)
					Expect(getCmd).To(RunSuccessfullyWithOutputString(MatchYAML(exp0 + exp0 + exp1)))
				})
				It("deletes a single mapping fifo", func() {
					deleteCmd := eksctlDeleteCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", role,
					)
					Expect(deleteCmd).To(RunSuccessfully())

					getCmd := eksctlGetCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,

						"--role", role,
						"-o", "yaml",
					)
					Expect(getCmd).To(RunSuccessfullyWithOutputString(MatchYAML(exp0 + exp1)))
				})
				It("fails when deleting unknown mapping", func() {
					deleteCmd := eksctlDeleteCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", "idontexist",
					)
					Expect(deleteCmd).ToNot(RunSuccessfully())
				})
				It("deletes duplicate mappings with --all", func() {
					deleteCmd := eksctlDeleteCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", role,
						"--all",
					)
					Expect(deleteCmd).To(RunSuccessfully())

					getCmd := eksctlGetCmd.WithArgs(
						"iamidentitymapping",
						"--name", clusterName,
						"--role", role,
						"-o", "yaml",
					)
					Expect(getCmd).ToNot(RunSuccessfully())
				})
			})

			Context("and delete the second nodegroup", func() {
				It("should not return an error", func() {
					cmd := eksctlDeleteCmd.WithArgs(
						"nodegroup",
						"--verbose", "4",
						"--cluster", clusterName,
						testNG,
					)
					Expect(cmd).To(RunSuccessfully())
				})

				It("{FLAKY: https://github.com/weaveworks/eksctl/issues/717} should make it 4 nodes total", func() {
					test, err := newKubeTest()
					Expect(err).ShouldNot(HaveOccurred())
					defer test.Close()

					test.WaitForNodesReady(4, commonTimeout)

					nodes := test.ListNodes((metav1.ListOptions{
						LabelSelector: api.NodeGroupNameLabel + "=" + initNG,
					}))
					allNodes := test.ListNodes((metav1.ListOptions{}))
					Expect(len(nodes.Items)).To(Equal(4))
					Expect(len(allNodes.Items)).To(Equal(4))
				})
			})
		})

		Context("and scale the initial nodegroup back to 1 node", func() {
			It("should not return an error", func() {
				cmd := eksctlScaleNodeGroupCmd.WithArgs(
					"--cluster", clusterName,
					"--nodes", "1",
					"--name", initNG,
				)
				Expect(cmd).To(RunSuccessfully())
			})

			It("{FLAKY: https://github.com/weaveworks/eksctl/issues/717} should make it 1 nodes total", func() {
				test, err := newKubeTest()
				Expect(err).ShouldNot(HaveOccurred())
				defer test.Close()

				test.WaitForNodesReady(1, commonTimeout)

				nodes := test.ListNodes((metav1.ListOptions{
					LabelSelector: api.NodeGroupNameLabel + "=" + initNG,
				}))

				Expect(len(nodes.Items)).To(Equal(1))
			})
		})

		Context("and deleting the cluster", func() {

			It("{FLAKY: https://github.com/weaveworks/eksctl/issues/536} should not return an error", func() {
				if !doDelete {
					Skip("will not delete cluster " + clusterName)
				}

				cmd := eksctlDeleteClusterCmd.WithArgs(
					"--name", clusterName,
					"--wait",
				)
				Expect(cmd).To(RunSuccessfully())
			})

			It("{FLAKY: https://github.com/weaveworks/eksctl/issues/536} should have deleted the EKS cluster and both CloudFormation stacks", func() {
				if !doDelete {
					Skip("will not delete cluster " + clusterName)
				}

				awsSession := NewSession(region)

				Expect(awsSession).ToNot(HaveExistingCluster(clusterName, awseks.ClusterStatusActive, version))

				Expect(awsSession).ToNot(HaveExistingStack(fmt.Sprintf("eksctl-%s-cluster", clusterName)))
				Expect(awsSession).ToNot(HaveExistingStack(fmt.Sprintf("eksctl-%s-nodegroup-ng-%d", clusterName, 0)))
			})
		})
	})
})
