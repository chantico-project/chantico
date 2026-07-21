---
title: "Contributing with DCO sign-off"
weight: 35
---

Contributions to Chantico requires Developer's Certificate of origin (DCO), as stated in our [contribution guide](https://github.com/chantico-project/.github/blob/main/CONTRIBUTING.md#developer-certificate-of-origin). This document describes several ways to handle DCO sign-off with git for developers wishing to contribute to the Chantico project.

---

## Using DCO sign-off

### Git CLI

This can be configured with git CLI in at least the following ways:

1. **Plain option:** Use the default `git commit -s` command when committing.
2. **Commit message hook:** In order to be able to commit with `commit -m` while also using DCO, add a git message hook for this cloned repository to add the sign-off automatically. Add the following lines to your local repository's `.git/hooks/commit-msg`:

```bash
#!/bin/sh
SIGNATURE="Signed-off-by: `git config --global --get user.name` <`git config --global --get user.email`>"
grep -qs "^${SIGNATURE}" "$1" || echo "\n${SIGNATURE}" >> "$1"
```
3. **Git aliases:** Another option to configure this automatically is with git aliases (shortcuts). In this way you can add the `-s` field to the commit command. Add the following lines to your `~/.gitconfig`.

```
[alias]
  cmsg = commit -s -m
  camend = commit -s --amend
```

To add sign-offs retroactively, use git rebase with signoff option, like so `git rebase --signoff HEAD^^`. Use as many `^` as there are commits in your pull requests. If the commits are already pushed, a force push should be performed afterwards.

### VSCode

When using git from the VSCode IDE there is an option to include DCO sign-off by setting the option "Git: Always Sign Off" (`alwaysSignOff`), either for user or workspace.