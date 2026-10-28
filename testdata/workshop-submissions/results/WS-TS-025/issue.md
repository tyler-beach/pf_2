# Tekton pipeline auth documentation [tech_spec]

<!-- submission_kind: tech_spec -->
<!-- provenance:
  source_url: https://github.com/tektoncd/pipeline/blob/main/docs/auth.md
  repo: tektoncd/pipeline
  path: docs/auth.md
  license: Apache-2.0
  retrieved: 2026-10-28
  excerpt_sha256_12: 6c9115ff1d0a
-->

## Design context

Workshop body-of-evidence submission sourced from a public third-party repository.
`submission_kind` is **tech_spec**. Domains: ci, auth.

## Technical specification / ADR (excerpt)

<!--
---
linkTitle: "Authentication"
weight: 301
---
-->

# Authentication at Run Time

This document describes how Tekton handles authentication when executing
`TaskRuns` and `PipelineRuns`. Since authentication concepts and processes
apply to both of those entities in the same manner, this document collectively
refers to `TaskRuns` and `PipelineRuns` as `Runs` for the sake of brevity.

**What this document covers:** How to provide credentials to the processes that
run *inside* the Pod created by a Run. These credentials allow Steps to perform
actions such as cloning from a private Git repository, pushing images with `kaniko`,
pulling images with `podman`, or performing OCI operations with `skopeo`.

**What this document does NOT cover:** Pulling the Step container images themselves
from a private registry. That happens on the Node when the Pod is scheduled, before
any Step runs. Use Kubernetes
[`imagePullSecrets`](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/)
on the ServiceAccount for that. See the Kubernetes documentation:
[Configure a Pod to Use a Private Registry](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/).

- [Overview](#overview)
- [Understanding credential selection](#understanding-credential-selection)
- [Using `Secrets` as a non-root user](#using-secrets-as-a-non-root-user)
- [Limiting `Secret` access to specific `Steps`](#limiting-secret-access-to-specific-steps)
- [Configuring authentication for Git](#configuring-authentication-for-git)
  - [Configuring `basic-auth` authentication for Git](#configuring-basic-auth-authentication-for-git)
  - [Configuring `ssh-auth` authentication for Git](#configuring-ssh-auth-authentication-for-git)
  - [Using a custom port for SSH authentication](#using-a-custom-port-for-ssh-authentication)
  - [Using multiple SSH credentials for different repositories on the same host](#using-multiple-ssh-credentials-for-different-repositories-on-the-same-host)
  - [Using SSH authentication in `git` type `Tasks`](#using-ssh-authentication-in-git-type-tasks)
- [Configuring authentication for Docker](#configuring-authentication-for-docker)
  - [Configuring `basic-auth` authentication for Docker](#configuring-basic-auth-authentication-for-docker)
  - [Configuring `docker*` authentication for Docker](#configuring-docker-authentication-for-docker)
- [Technical reference](#technical-reference)
  - [`basic-auth` for Git](#basic-auth-for-git)
  - [`ssh-auth` for Git](#ssh-auth-for-git)
  - [`basic-auth` for Docker](#basic-auth-for-docker)
  - [Errors and their meaning](#errors-and-their-meaning)
    - ["unsuccessful cred copy" Warning](#unsuccessful-cred-copy-warning)
      - [Multiple Steps with varying UIDs](#multiple-steps-with-varying-uids)
      - [A Workspace or Volume is also Mounted for the same credentials](#a-workspace-or-volume-is-also-mounted-for-the-same-credentials)
      - [A Task employes a read-only-Workspace or Volume for `$HOME`](#a-task-employs-a-read-only-workspace-or-volume-for-home)
      - [The Step is named `image-digest-exporter`](#the-step-is-named-image-digest-exporter)
- [Disabling Tekton's Built-In Auth](#disabling-tektons-built-in-auth)
  - [Why would an organization want to do this?](#why-would-an-organization-want-to-do-this)
  - [What are the effects of making this change?](#what-are-the-effects-of-making-this-change)
  - [How to disable the built-in auth](#how-to-disable-the-built-in-auth)

## Overview

