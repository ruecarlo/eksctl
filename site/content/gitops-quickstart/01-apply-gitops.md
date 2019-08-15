---
title: Setup your cluster with GitOps
weight: 10
url: gitops-quickstart/setup-gitops
---

# Setup your cluster with GitOps

Welcome to `eksctl` GitOps Quick Starts. This will allow you to launch
fully-configured Kubernetes clusters that are ready to run production
workloads in minutes: easy for you to get started running Kubernetes
on EKS and to launch standard clusters in your organisation.

At the end of this guide, you will have a Kubernetes cluster including
control plane, worker nodes, and all of the software needed for code
deployment, monitoring, and logging.

## Quick Start to GitOps

[GitOps][gitops] is a way to do Kubernetes application delivery. It
works by using Git as a single source of truth for Kubernetes resources
and everything else. With Git at the center of your delivery pipelines,
you and your team can make pull requests to accelerate and simplify
application deployments and operations tasks to Kubernetes.

[gitops]: https://www.weave.works/technologies/gitops/

Using GitOps Quick Starts will get you set up in next to no time. You
will benefit from a setup that is based on the experience of companies
who run workloads at scale.

## Prerequisites

To use EKS, you need to have your [AWS account][aws-account] set up.

Next you will have to have the following tools installed:

- [AWS CLI][aws-cli]: at least `1.16.156` - older versions will require
  [AWS IAM Authenticator][aws-iam-authenticator] to be installed too
- a specific version of [kubectl][aws-kubectl] which works with
  EKS

[aws-account]: https://aws.amazon.com/account/
[aws-cli]: https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-install.html
[aws-iam-authenticator]: https://github.com/kubernetes-sigs/aws-iam-authenticator
[aws-kubectl]: https://docs.aws.amazon.com/eks/latest/userguide/install-kubectl.html

### Getting ready for GitOps

The main point of GitOps is to keep everything (config, alerts, dashboards,
apps, literally everything) in git and use it as a single source of truth.
To keep your cluster configuration in git, please go ahead and create an
_empty_ repository. On Github for example, follow [these steps][github-repo].

[github-repo]: https://help.github.com/articles/create-a-repo

## Standing up your cluster

First we follow the [usual steps](/introduction/getting-started/) to stand
up a cluster on EKS. Please review the list of flags to see if you need to
tweak it for your purposes. In essence it is going to be a variation of:

```console
eksctl create cluster
```

Once it is finished, you should be able to check the cluster contents and
see some system workloads:

```console
$ kubectl get pods --all-namespaces
NAMESPACE     NAME                      READY   STATUS    RESTARTS   AGE
kube-system   aws-node-cl5t5            1/1     Running   0          1m
kube-system   aws-node-k96bc            1/1     Running   0          1m
kube-system   coredns-d5c56458d-wc68z   1/1     Running   0          9m
kube-system   coredns-d5c56458d-zz8d6   1/1     Running   0          9m
kube-system   kube-proxy-d577n          1/1     Running   0          3m
kube-system   kube-proxy-tbmdd          1/1     Running   0          3m
$
```

## Applying GitOps

The following command will set up your cluster with the `app-dev` profile,
the first GitOps Quick Start. All of the config files you need for a
production-ready cluster will be in the git repo you have provided and
those components will be deployed to your cluster. When you make changes
in the configuration they will be reflected on your cluster.

> This is an experimental feature. To enable it, set the environment
> variable `EKSCTL_EXPERIMENTAL=true`.
>
> Experimental features are not stable and their command name and flags
> may change.

```console
EKSCTL_EXPERIMENTAL=true eksctl \
        gitops apply app-dev-profile \
        git-url=https://github.com/example/my-eks-config
```

- xxx: What will be installed on the cluster as part of this command

This will set up Flux on your cluster and load GitOps Quick Start config
files into your repo. It will use templating to add your cluster name and
region to the configuration so that key cluster components that need those
values can work (e.g. `alb-ingress`).

## Your GitOps cluster

- xxx: Describe what happens there
- xxx: How users should check what is running at the end of the process
  (e.g. get pods, port forward to kube dashboard, grafana, how they get
  to the demo app that has been deployed)

## Advanced setups

`eksctl gitops apply` can largely be decomposed into

1. `eksctl install flux`
1. `eksctl generate config`

So for more complex use cases, you will want to run these steps
separately and on your own and adjust flags and options as necessary. The
first command installs Flux and links it to a git repo that you provide.
The second generates the config files from the GitOps Quick Start profile
locally, so that you can edit them before pushing to your git repo.

### Configuring Flux

Here we will install [Flux](https://fluxcd.io), the Kubernetes GitOps
operator in your cluster. It will take care of deployments for you.

```console
EKSCTL_EXPERIMENTAL=true eksctl install flux \
    --git-url <git-url> \
    --git-email <email-of-committer> \
    --name <cluster-name>
```

Additional options to the command are explained in our docs on
[`install flux`](/usage/experimental/gitops-flux/).

After about a minute your cluster will have `flux` running, which will
monitor your git repository once you added a deploy key to e.g. Github.

The Flux Helm operator and Tiller will be deployed in your cluster
too at this point. (XXX: pending
<https://github.com/weaveworks/eksctl/pull/1136>).

This key can be found at the end of the output of the command, this
might for example be:

```console
$ EKSCTL_EXPERIMENTAL=true eksctl install flux \
    --git-url git@github.com:happy-gopher/flux-get-started \
    --git-email happy@gopher.org \
    --name wonderful-wardrobe-1565767990
[...]
[ℹ]  Flux will only operate properly once it has write-access to the Git repository
[ℹ]  please configure git@github.com:happy-gopher/flux-get-started so that the following Flux SSH public key has write access to it
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCt8no0/F3+kD1YukH6sVIv1ONcy9+01G2/AQe1CQA+uRHaioep41U3ghROU7CoM1yTjG+eLYgu26UMvkXVbOmEm+1697adh4qz/yCF0E7JtCIIXGn/1XrLb6OxgtlGKdJ4fTUdxtQSyTvWqjxQhC4ute9hnHWU8oiSrNaq5D20P5x8sgPf4V0A5YWD5S4YliJcIupTzrD7zjhh6TyP5fqhPLHPBZFHStHq0DSD+Gi6vXZz1s9UmuAnxP8pkIlrW22xJyFbsmcjJuks5FvmLo8uJMeWTx5t+3WKWp8ZKrbDJFUWQ8aVMByHYq1c3doevM28CHwz/
```

Copy the lines starting with `ssh-rsa` and add it as a deploy key, to
e.g. Github. There you can easily do this in the
`Settings > Deploy keys > Add deploy key`. Just make sure you check
`Allow write access` as well.

The next time Flux syncs from git, it will start updating the cluster
and actively deploying. If you use the [`flux-get-started`
repo](https://github.com/fluxcd/flux-get-started) from above, here's
what you will see in your cluster:

```console
$ kubectl get pods -n demo
NAME                       READY   STATUS    RESTARTS   AGE
podinfo-5f4bd464b4-4vf7k   1/1     Running   0          58m
podinfo-5f4bd464b4-hkgzt   1/1     Running   0          58m
```

Remember that you can further tweak the installation of Flux as
discussed in our [`install flux` docs](/usage/experimental/gitops-flux/).

### Handcrafting your configuration

In this second step we will use `eksctl generate config`, so you can
easily handcraft your cluster configuration and maintain it in Git, just
like you would for the workloads you are running.

During the previous command (`eksctl install flux`), we instructed Flux
to watch a config repository. This is where the workloads are defined.
Now we will add the cluster config as well.

In this example we will run:

```console
EKSCTL_EXPERIMENTAL=true eksctl generate profile \
        --name wonderful-wardrobe-1565767990 \
        --git-url git@github.com:happy-gopher/eks-gitops-example.git \
        --profile-path ~/dev/flux-get-started/cluster-config
```

Let's break this down here. `eksctl generate profile` at the very
least wants:

- `--name`: the name of the cluster - check `eksctl get cluster`
  to see what the name of yours is
- `--git-url`: the Git URL of the base config of your cluster
- `--profile-path`: a local path: this is an empty new directory
  (here `cluster-config`) you create in your local checkout of
  the config repository, which we used in the previous command

The base config can be something you and your organisation
tailored to your needs, but can be something like our [EKS
GitOps example][eks-gitops-example] as well. It is meant to be
a starting point for clusters you can iterate over.

[eks-gitops-example]: https://github.com/weaveworks/eks-gitops-example

Check `XXX` for a description of all the flags and options -
blocked on <https://github.com/weaveworks/eksctl/pull/1140>.

So after all this preface, what happens when we run the command?
`eksctl` will check out the base-config (here we use a fork of the
GitOps example config) in an empty sub-directory (`cluster-config`)
of our local checkout of our `flux-get-started` fork.

All that is left now to get our cluster components managed by Flux
is to commit them to our config repository:

```console
cd ~/dev/flux-get-started/cluster-config
git add .
git commit -m "add cluster config"
git push
```

Flux will pick this up in its next sync and make the changes to your
cluster.

In our case we are going to see these new arrivals in the cluster:

```console
kubectl get pods -n kubernetes-dashboard
kubernetes-dashboard   dashboard-metrics-scraper-f7b5dbf7d-kwz7n   1/1     Running   0          4m
kubernetes-dashboard   kubernetes-dashboard-7447f48f55-2pl66       1/1     Running   0          4m
```

All of the cluster configuration can be easily edited in git now.
Welcome to a fully GitOpsed world!

- xxx: creating your own QuickStart style repo and give
  that as an argument to eksctl gitops apply or eksctl generate config

## Conclusion

- xxx: Next steps

We look forward to hearing your thoughts and feedback. Please [get
in touch](/community/get-in-touch/) and let us know how things
worked out for you.
