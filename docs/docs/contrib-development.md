---
title: Development
---

This page details the process for getting up and running locally for OPA
development. If you're a first time contributor, we recommend you read through
the [Contributing to OPA](./contrib-code) page first.

OPA is written in the [Go](https://golang.org) programming language.
If you are new to Go, consider reading
[Effective Go](https://golang.org/doc/effective_go.html),
[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) or
[How to Write Go Code](https://go.dev/doc/code)
for guidance on writing idiomatic Go code.

Requirements:

- Git
- GitHub account (if you are contributing)
- Go (please see the project's [go.mod](https://github.com/IUAD1IY7/opa/blob/main/go.mod) file for the current version in use)
- GNU Make
- Python3, pip, yamllint (if linting YAML files manually)

## Getting Started

After forking the repository and creating a [clone from your fork](https://docs.github.com/en/get-started/quickstart/contributing-to-projects),
just run `make`. This will:

- Build the OPA binary.
- Run all of the tests.
- Run all of the static analysis checks.

If the build was successful, a binary will be produced in the top directory (`opa_<OS>_<ARCH>`).

Verify the build was successful with `./opa_<OS>_<ARCH> run`.

You can re-build the project with `make build`, execute all of the tests
with `make test`, and execute all of the performance benchmarks with `make perf`.

For quicker development-test iteration, you may use `make test-short` during development,
and only run `make test` before submitting your changed. This avoids running the slowest
tests and normally completes under a minute (compared to the several minutes required to run
the full test suite).

The static analysis checks (e.g., `go fmt`, `golint`, `go vet`) can be run
with `make check`.

> To correct any imports or style errors run `make fmt`.

## Workflow

### Fork, clone, create a branch

Go to [https://github.com/IUAD1IY7/opa](https://github.com/IUAD1IY7/opa) and fork the repository
into your account by clicking the "Fork" button.

Clone the fork to your local machine:

```bash
git clone git@github.com/<GITHUB USERNAME>/opa.git opa
cd opa
git remote add upstream https://github.com/IUAD1IY7/opa.git
```

Create a branch for your changes.

```bash
git checkout -b somefeature
```

### Developing your change

Develop your changes and regularly update your local branch against upstream,
for example by rebasing:

```bash
git fetch upstream
git rebase upstream/main
```

> Be sure to run `make check` before submitting your pull request. You
> may need to run `go fmt` on your code to make it comply with standard Go
> style.
> For YAML files, you may need to run the `yamllint` tool on the
> `test/cases/testdata` folder to make sure any new tests are well-formatted.
> If you have Docker available, you can run `make check-yaml-tests` to
> run `yamllint` on the tests without installing any Python dependencies.

### Submission

Commit changes and push to your fork.

```bash
git commit -s
git push origin somefeature
```

> Make sure to use a [good commit message](./contrib-code/#commit-messages).

Now, submit a Pull Request from your fork.
See the official [GitHub Documentation](https://help.github.com/en/github/collaborating-with-issues-and-pull-requests/creating-a-pull-request-from-a-fork)
for instructions to create the request.

> Hint: You should be prompted to with a "Compare and Pull Request" button
> that mentions your new branch on [https://github.com/IUAD1IY7/opa](https://github.com/IUAD1IY7/opa)

Once your Pull Request has been reviewed and signed off please squash your
commits. If you have a specific reason to leave multiple commits in the
Pull Request, please mention it in the discussion.

> If you are not familiar with squashing commits, see [the following blog post for a good overview](http://gitready.com/advanced/2009/02/10/squashing-commits-with-rebase.html).

## Benchmarks

Several packages in this repository implement benchmark tests. To execute the
benchmarks you can run `make perf` in the top-level directory. We use the Go
benchmarking framework for all benchmarks.

## Dependencies

OPA is a Go module [https://github.com/golang/go/wiki/Modules](https://github.com/golang/go/wiki/Modules)
and dependencies are tracked with the standard [go.mod](https://github.com/IUAD1IY7/opa/blob/main/go.mod) file.

We also keep a full copy of the dependencies in the [vendor](https://github.com/IUAD1IY7/opa/tree/main/vendor)
directory. All `go` commands from the [Makefile](https://github.com/IUAD1IY7/opa/blob/main/Makefile) will enable
module mode by setting `GO111MODULE=on GOFLAGS=-mod=vendor` which will also
force using the `vendor` directory.

To update a dependency ensure that `GO111MODULE` is either on, or the repository
qualifies for `auto` to enable module mode. Then simply use `go get ..` to get
the version desired. This should update the [go.mod](https://github.com/IUAD1IY7/opa/blob/main/go.mod) and (potentially)
[go.sum](https://github.com/IUAD1IY7/opa/blob/main/go.sum) files. After this you _MUST_ run `go mod vendor` to ensure
that the `vendor` directory is in sync.

Example workflow for updating a dependency:

```bash
go get -u github.com/sirupsen/logrus@v1.4.2  # Get the specified version of the package.
go mod tidy                                  # (Somewhat optional) Prunes removed dependencies.
go mod vendor                                # Ensure the vendor directory is up to date.
```

If dependencies have been removed ensure to run `go mod tidy` to clean them up.

### Go

If you need to update the version of Go used to build OPA you must update these
files in the root of this repository:

- `.go-version`- which is used by the Makefile and CI tooling. Put the exact go
  version that OPA should use.

## Refactoring and Style Fixes

If you've found some code that you think would benefit from a refactoring — either by making it more readable or more
performant, that's great! Some things should however be considered before you submit such a change:

- Avoid mixing bug fixes and feature PRs with refactorings or style fixes. These PRs are generally difficult to review.
  Instead, split your work up in multiple, separate PRs. If a refactoring is "needed" for a feature, at least ensure to
  split the two into separate commits.
- If you intend to work on a larger refactoring project, make sure to first create an issue for discussion. Sometimes
  things are the way they are for a reason, even when it's not immediately obvious.
- Ensure that there are tests covering the code subject to change.

## CI Configuration

OPA uses Github Actions defined in the [.github/workflows](https://github.com/IUAD1IY7/opa/tree/main/.github/workflows)
directory.

### Github Action Secrets

The following secrets are used by the Github Action workflows:

| Name                       | Description                                                                                                                                                     |
| -------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| S3_RELEASE_BUCKET          | AWS S3 Bucket name to upload `edge` release binaries to. Optional -- If not provided the release upload steps are skipped.                                      |
| AWS_ACCESS_KEY_ID          | AWS credentials required to upload to the configured `S3_RELEASE_BUCKET`. Optional -- If not provided the release upload steps are skipped.                     |
| AWS_SECRET_ACCESS_KEY      | AWS credentials required to upload to the configured `S3_RELEASE_BUCKET`. Optional -- If not provided the release upload steps are skipped.                     |
| DOCKER_IMAGE               | Full docker image name (with org) to tag and publish OPA images. Optional -- If not provided the image defaults to `openpolicyagent/opa`.                       |
| DOCKER_WASM_BUILDER_IMAGE  | Full docker image name (with org) to tag and publish WASM builder images. Optional -- If not provided the image defaults to `openpolicyagent/opa-wasm-builder`. |
| DOCKER_USER                | Docker username for uploading release images. Will be used with `docker login`. Optional -- If not provided the image push steps are skipped.                   |
| DOCKER_PASSWORD            | Docker password or API token for the configured `DOCKER_USER`. Will be used with `docker login`. Optional -- If not provided the image push steps are skipped.  |
| SLACK_NOTIFICATION_WEBHOOK | Slack webhook for sending notifications. Optional -- If not provided the notification steps are skipped.                                                        |
| TELEMETRY_URL              | URL to inject at build-time for OPA version reporting. Optional -- If not provided the default value in OPA's source is used.                                   |
| NETLIFY_BUILD_HOOK_URL     | URL to trigger Netlify (openpolicyagent.org) deploys after release. Optional -- If not provided the Netlify steps are skipped.                                  |

### Periodic Workflows

Some of the Github Action workflows are triggered on a schedule, and not included in the
post-merge, pull-request, etc actions. These are reserved for time consuming or potentially
non-deterministic jobs (race detection tests, fuzzing, etc).

Below is a list of workflows and links to their status:

| Workflow                                                                                                                                                                    | Description                    |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------ |
| [![Nightly](https://github.com/IUAD1IY7/opa/workflows/Nightly/badge.svg?branch=main)](https://github.com/IUAD1IY7/opa/actions?query=workflow%3A"Nightly") | Runs once per day at 8:00 UTC. |
