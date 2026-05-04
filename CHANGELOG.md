## 0.5.6 (2026-04-17)

#### Bug Fixes

* Rename finalizer of datacenter resource (3336af53)

#### Chores

* Rename GitHub organization to chantico-project (fabcd75d)
* **docs:** Update installation links and more future milestone content (dc8de4c9)


## 0.5.5 (2026-04-14)

#### Bug Fixes

* Publish release changelog to GitHub via semantic-release (1afed199)
* Make crds before packaging helm. (a2ce90c2)


## 0.5.4 (2026-04-14)

#### Bug Fixes

* move organisation labels from builder image to final image (b64e6823)


## 0.5.3 (2026-04-14)

#### Bug Fixes

* Dockerfile organisation labels and removal of image pull regcred (8ac1dacb)


## 0.5.2 (2026-04-13)

#### Bug Fixes

* Explicit helm versioning and correct registry paths. (764faabe)


## 0.5.1 (2026-04-13)

#### Bug Fixes

* Typo's and documentation updates (1d352f83)

#### Chores

* Write roadmap in documentation with use-cases and limitations (0eaefd17)


## 0.5.0 (2026-03-31)

#### Feature

* Build chantico recording rules logic into datacenterresource (45eb6acb)

#### Bug Fixes

* Context needed in action function of datacenterresource (9a4bcc3e)


## 0.4.5 (2026-03-31)

#### Bug Fixes

* unchange snmp root access. (5025721b)
* update rbac. (261709d0)
* add warning in docs. (7cc4898b)

#### Documentation

* add additional text in installment guide. (84cd8908)
* add getting started section install page. (41f142ed)

#### Chores

* remove dev pvc. (2d8ff776)
* packages upgrade. (e4619073)
* change filebrowser database path. (215263d3)
* remove hardcoded namespace naming. (c2b4832b)
* remove (part of) unused kustomize code. (ec73189e)
* add permissions for chantico in cluster. (56677396)

#### CI

* Goreleaser build separation, fix Chart.yaml path, inject Docker Hub token (44b6f7ed)


## 0.4.4 (2026-03-30)

#### Bug Fixes

* adds Chantico gopher back to README.md (ec5b8a34)

#### Documentation

* Correct relative content directory (d732d12b)

#### Chores

* **cleanup:** Initial restructure of hugo and docs (9eac3f58)
* update packages. (8951ff83)


## 0.4.3 (2026-03-20)

#### Bug Fixes

* **ci:** Avoid redundant changelog entry and publish changelog to GitHub (0e6c069c)

#### Chores

* **release:** bump version to v0.4.2 [skip ci] (d4c50a47)


## 0.4.2 (2026-03-20)

#### Bug Fixes

* **release:** Correct GitLab URLs (aa17dcb7)

## 0.4.1 (2026-03-20)

#### Bug Fixes

* **ci:** Correct syntax for file existence check in release job (4f2d2345)
* **ci:** Use goreleaser image for release job (f53e34ed)
* **ci:** Improvements for release, with changelog and version bumps (ad49cf6b)

#### Documentation

* Update links, menus, and add more sections and add link checker to CI (ca0829e0)
* Link architecture figure from use cases (9a0b86b1)

#### Chores

* update docs (3b12f9b8)


## 0.4.0 (2026-03-13)

#### Feature

* replace Prometheus config merging with service discovery (67ce0300)

#### Chores

* Clean up old Postgres-specific code for storing measurements (76264d03)


## 0.3.0 (2026-03-05)

#### Feature

* seperate method and write unit test. (7c067f56)
* implement polling of Prometheus to regulary check endpoints. (8a914bdd)


## 0.2.3 (2026-03-05)

#### Bug Fixes

* Correct links in API reference (a2272083)

#### Documentation

* Improve license/URLs (67b54845)

#### Chores

* Upgrade minimum Go version to 1.24.13 and default to 1.25.8 (a8c23b5a)
* Fix formatting of webapp files (6e3dae28)

#### CI

* Fix link style variable (c2a4ffca)
* Run test on tag job (5d6b5a2e)


## 0.2.2 (2026-02-26)

#### Bug Fixes

* Resolve merge conflicts. (9acfa575)
* show error instead of status message for Prometheus reloading. (523ae569)
* resolve giving r/w permissions per folder. (63d40288)


## 0.2.1 (2026-02-26)

#### Bug Fixes

* Do not validate convergent parents in data center resource graph (fecfb9a9)

#### Documentation

* Add namespace to kubectl command (3bcb8717)

#### Tests

* Add unit test for self-reference in data center resource (4b655290)

#### Chores

* Update variable name of data center resource (bcf3c2d9)

#### CI

* Token from variables instead (9994f589)
* Commit change to helm chart before goreleaser (3a12aa88)


## 0.2.0 (2026-02-26)

#### Feature

* improve k8s installation docs (fb4f7afe)
* wait in migration job and update goose image for better encryption (7dcabb02)
* Correct postgres service selector and goose migration path within pod (d9aa9a30)
* Add image pull secret and docs on setting up token in GitLab (331b2378)
* correct job namespace (4b975958)
* correct syntax and add hook annotation (8d0f04b8)
* job to perform goose migration and wait for postgres to be up in initcontainer (bebf961c)
* cache layers during build, build some images only on main branch (1d018fac)

#### Bug Fixes

* **docs:** Correct indentation of the code blocks in local setup doc (2a92fea8)
* re-introduce SetupWithManager (ef7da1f2)

#### Documentation

* Update README.md chantico description (9edfe8f4)
* Update README.md with contributing/code/docs, add LICENSE file (8c8abf9b)
* Add autogeneration of how-to list (113b82c3)

#### Chores

* Remove old persistent claim volume on teardown (bc5259db)

#### CI

* Handle semantic-release exit code gracefully (861cfa75)
* No CI for manual release test job (8cf3ad0c)
* Provide branch name for manual release test job (d1283bc1)
* Allow running manual test without deps (8eca9b55)
* Make manual job for build release test more like release (69cf1e54)
* Make separate line for GitLab and improve config (1cad91e8)
* Change image for build test (df0f6ac5)
* Test build on non-main branches (7cb172fd)
* Initial configuration of release pipeline (f74fc2ef)
* Remove missing dependency (135e6d4d)
* Set up release pipeline (a1057ca8)
* Provide scanner job ID for artifact URL in deploy (955c0d24)
* Deploy scan even if job fails (1c2e8cda)
* Adjust stage dependencies, allow scan failure for deployment (9a737224)
* Make lint steps a warning and indicate all outputs/files involved (b90223c0)
* Move fmt/vet recipes to run only in dedicated stage (3eb8a9a1)
* Add preview build of documentation for merge request (5d4389b8)
* build chantico aggregator as well (1ccbd7ac)
* add build/publish stage for main chantico image (b98de476)
* update command information (59fc9624)
* add chantico-goose build (3033941d)

#### design

* change os.Setenv to t.Setenv (6c866854)
* remove outdated test folder based on the operator-sdk (85f03fa0)
* document the testing approach (8ac7db10)
* run all tests in CI and track total coverage (0a8e9088)
* add create SNMP generator (546cae37)
* fix yaml indentation (18d6725e)
* rewrite TestGetState (a90c9a65)
* remove end-to-end testing (34d4488c)
* start testing the interaction with the snmp exporter (8a7e9f5a)
* delegate volume creation to its own package (62de24fc)
* add used docker images to its own package (77fc2341)
* implement requeue with delay (55869db3)
* improve test (13409d55)
* add test report and adjust coverage pattern (b5805b0d)
* Add CI test stage for measurementdevice tests (4e764d50)
* remove duplicate test (fb8f74a0)
* Format tests with subtests and failure messages (b0c171ab)
* add nil case for measurementDevice (cbd35af7)
* clean up old structure (9a4afe89)
* add test to check the ActionMap (18b32d2b)
* regroup actions and state of measurementdevice (c74e915f)
* move actions to a separate module (fa96f284)
* initial concept of action tests (a4a2480b)

#### readme

* Mention local registry (8a3cde65)

#### docker

* Increase Go version to 1.23 for `go mod download` (45b6f7ea)

#### bug

* fix race condition on redeployment (and more) (fa4e6b59)


## 0.1.0 (2025-07-11)

### Bug Fixes

* change master to main (bd28ff0)
* json format in .releaserc.json (1c54aed)

### Features

* add ci (0146922)
