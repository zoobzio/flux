# Contributing to flux

Thank you for your interest in contributing to flux! This guide will help you get started.

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment for all contributors.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/yourusername/flux.git`
3. Create a feature branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Run tests: `make test`
6. Run linters: `make lint`
7. Commit your changes with a descriptive message
8. Push to your fork: `git push origin feature/your-feature-name`
9. Create a Pull Request

## Development Guidelines

### Code Style

- Follow standard Go conventions
- Run `go fmt` before committing
- Add comments for exported functions and types
- Keep functions small and focused

### Testing

- Write tests for new functionality
- Ensure all tests pass: `make test`
- Maintain 1:1 file-to-test ratio
- Aim for 80%+ test coverage

### Documentation

- Update README.md for API changes
- Add comments to all exported types
- Keep doc comments clear and concise

## Types of Contributions

### Bug Reports

- Use GitHub Issues
- Include minimal reproduction code
- Describe expected vs actual behavior
- Include Go version and OS

### Feature Requests

- Open an issue for discussion first
- Explain the use case
- Consider backwards compatibility

### Code Contributions

All contributions should:
- Include comprehensive tests
- Pass linter checks
- Maintain existing code style
- Update documentation as needed

## Pull Request Process

1. **Keep PRs focused** - One feature/fix per PR
2. **Write descriptive commit messages**
3. **Update tests and documentation**
4. **Ensure CI passes**
5. **Respond to review feedback**

## Testing

Run the full test suite:
```bash
make test
```

Run with coverage:
```bash
make coverage
```

Run linters:
```bash
make lint
```

Run full CI simulation:
```bash
make ci
```

## Project Structure

```
flux/
├── *.go              # Core library files
├── *_test.go         # Tests (1:1 with source files)
├── .github/          # GitHub workflows and templates
├── docs/             # Documentation
├── examples/         # Example applications
├── testing/          # Integration tests and benchmarks
├── README.md         # Project documentation
└── Makefile          # Build and test commands
```

## Commit Messages

Follow conventional commits:
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `test:` Test additions/changes
- `refactor:` Code refactoring
- `perf:` Performance improvements
- `chore:` Maintenance tasks

## Release Process

### Automated Releases

This project uses automated release versioning. To create a release:

1. Go to Actions > Release > Run workflow
2. Leave "Version override" empty for automatic version inference
3. Click "Run workflow"

The system will:
- Automatically determine the next version from conventional commits
- Create a git tag
- Generate release notes via GoReleaser
- Publish the release to GitHub

### Commit Conventions for Versioning
- `feat:` new features (minor version: 1.2.0 > 1.3.0)
- `fix:` bug fixes (patch version: 1.2.0 > 1.2.1)
- `feat!:` breaking changes (major version: 1.2.0 > 2.0.0)
- `docs:`, `test:`, `chore:` no version change

Example: `feat(capacitor): add retry option`

### Version Preview on Pull Requests
Every PR automatically shows the next version that will be created:
- Check PR comments for "Version Preview"
- Updates automatically as you add commits
- Helps verify your commits have the intended effect

## Questions?

- Open an issue for questions
- Check existing issues first
- Be patient and respectful

Thank you for contributing to flux!
