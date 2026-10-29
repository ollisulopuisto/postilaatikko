#!/usr/bin/env python3
"""Select affected CI jobs; unrecognized paths request both language suites."""
import argparse
import os
import pathlib
import subprocess


def classify(paths):
    """Return frontend/backend flags, ignoring only known documentation paths."""
    frontend = False
    backend = False
    for path in paths:
        if path.endswith('.go') or path in {'go.mod', 'go.sum', '.golangci.yml'}:
            backend = True
            continue
        if path.startswith('frontend/'):
            frontend = True
            continue
        if path.startswith(('docs/', '.github/ISSUE_TEMPLATE/')):
            continue
        if '/' not in path and (path.endswith('.md') or path == 'LICENSE'):
            continue
        return True, True
    return frontend, backend


def changed_paths(base, cwd=None):
    """Read all changed names, including both rename sides, without API limits."""
    result = subprocess.run(
        ['git', 'diff', '--no-renames', '--name-only', '-z', base, 'HEAD', '--'],
        cwd=cwd, check=True, stdout=subprocess.PIPE,
    )
    return [os.fsdecode(path) for path in result.stdout.split(b'\0') if path]


def main():
    """Print GitHub output flags; invalid Git ranges fail instead of skipping."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', required=True)
    args = parser.parse_args()
    frontend, backend = classify(changed_paths(args.base))
    output = f'frontend={str(frontend).lower()}\nbackend={str(backend).lower()}\n'
    print(output, end='')
    destination = os.environ.get('GITHUB_OUTPUT')
    if destination:
        with pathlib.Path(destination).open('a') as stream:
            stream.write(output)


if __name__ == '__main__':
    main()
