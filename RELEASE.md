# Release Process

## Releasing a major or minor version

**Prerequisites:**

* Install goreleaser:
    ```bash
    go install github.com/goreleaser/goreleaser@latest
    ```
**Steps:**

1. Determine the release version:

    -  Identify the `MAJOR`, `MINOR`, and `PATCH` version numbers for the release. 
    -  For example:
        ```bash
        MAJOR="1"
        MINOR="2"
        PATCH="3" 
        ```

2. Create a release branch:


    - Create a release branch using the format `release-${MAJOR}.${MINOR}`:

        ```bash
        git checkout -b release-${MAJOR}.${MINOR}
        ```

    - Push the release branch to the upstream repository:

        ```bash
        git push upstream release-${MAJOR}.${MINOR} 
        ```

3. Create and push a tag:

    - Create a new tag using the format `v${MAJOR}.${MINOR}.${PATCH}`:

        ```bash
        git tag v${MAJOR}.${MINOR}.${PATCH}
        ```

    - Push the release branch to the upstream repository:

        ```bash
        git push upstream v${MAJOR}.${MINOR}.${PATCH}
        ```

4. Build release artifacts:

    - Use `goreleaser` to build the release artifacts:

        ```bash
        goreleaser release --clean --skip-publish 
        ```

        This command generates the release artifacts in the `dist` directory.

5. **Create a GitHub release:**

    - Go to the project's GitHub repository and navigate to the [New Releases
      page](https://github.com/kubernetes-sigs/gwctl/releases/new).
    - Select the newly created tag (`v${MAJOR}.${MINOR}.${PATCH}`) from the "Choose a tag" dropdown.
    - Attach the relevant release artifacts (`.tar.gz` and `.zip` files) from the `dist` directory.
    - Publish the release.
