# Using agentic and generative AI in Chantico development

## Contributions

Chantico is an open-source project. Contributions must be licensed properly:

- Code contributions must be licensed under the [Apache 2.0 
  License](http://www.apache.org/licenses/LICENSE-2.0).
- Documentation contributions must be licensed under [CC-BY 4.0](https://creativecommons.org/licenses/by/4.0/deed.en).

For new Go code files, use the template in `hack/boilerplate.go.txt` to include 
the license header at the top of the file.

If agentic AI has output based on pre-existing content, avoid inclusion of code 
and documentation that is not compatible with these respective licenses.

## Setup

- Read the [local development environment setup 
  guide](how-to-setup-the-local-development-environment.md) to set up a local 
  kind cluster and install the Chantico deployment on it.
- Requirements: Go, kind, docker, helm, make and kubectl.
- Install: `./dev/setup.sh`
- Run the controllers locally: `make run`

## Testing

- Run from the root of the repository: `make test`
- Find CI plan in `.github/workflows` directory
- Test code format and linting: `make fmt` and `make vet`
- Read [coding style guidelines](coding-style-guidelines.md) for more details on 
  code formatting and style to apply during code generation.

## Pull Requests

- Always run tests and checks before creating a PR.
- [Conventional commit](https://www.conventionalcommits.org/en/v1.0.0/) format 
  is used for PR title and commit message: `<type>(<scope>): <Description>`, 
  where `type` is `feat` (new user-facing feature), `fix` (bug fixes) or `chore` 
  (other changes that do not affect the deployment of Chantico), and `scope` is 
  an optional indicator of the codebase affected by the change, such as 
  `<controller>`, `api`, `ci`, `docs` or `test` (but not limited to these). 
  Write description with initial capital letter and limit to ~50 characters.
- Refer to any issue that is resolved by the PR in the description, if any.
- Include in PR a reproducible execution flow, either as a test file or as 
  a list of steps to run in the PR description.
