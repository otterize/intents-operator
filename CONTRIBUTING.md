# Contributing to the Otterize Codebase

Thanks for considering contributing to Otterize! This document outlines the procedure for contributing new features
and bugfixes to Otterize. Following these steps will help ensure that your contribution gets merged quickly and
efficiently.

## Overview

### Is this a new feature?

If you'd like to add a new feature, please first open an issue describing the desired functionality. A maintainer will work with you to come up with the correct design for the new feature. Once you've agreed on a design, you can then start contributing code to the relevant Otterize repositories using the development process described below.

Remember, agreeing on a design in advance will greatly increase the chance that your PR will be accepted, and minimize the amount of time required for both you and your reviewer!

### Is this a simple bug fix?

Simple bug fixes can just be raised as a pull request. Make sure you describe the bug in the pull request description,
and please try to reproduce the bug in a test. This will help ensure the bug stays fixed!

### PR process

Once you've agreed on a design for your bugfix or new feature, development should be done using the following steps:

1. [Create a personal fork][fork] of the repository.
2. Pull the latest code from the **main** branch and create a feature branch off of this in your fork.
3. Implement your feature. Commits are cheap in Git; try to split up your code into many. It makes reviewing easier as well as for saner merging.
4. Make sure that existing tests are passing, and that you've written new tests for any new functionality. Each directory has its own suite of tests. 
5. Push your feature branch to your fork on GitHub.
6. [Create a pull request][pulls] using GitHub, from your fork and branch to the `main` branch.
    1. Opening a pull request will automatically run your changes through our CI. Make sure all tests pass so that a maintainer can merge your contribution.
7. Await review from a maintainer.
8. When you receive feedback:
    1. Address code review issues on your feature branch.
    2. Push the changes to your fork's feature branch on GitHub. This automatically updates the pull request.
    3. If necessary, make a top-level comment along the lines of “Please re-review”, notifying your reviewer, and repeat the above.
    4. Once your PR has been approved and the commits have been squashed, your reviewer will merge the PR.

