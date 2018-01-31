#!/usr/bin/env python2

"""
Jenkins does a great job computing line coverage and publishing it.  However,
packages with .go files but without _test.go files don't get included in
coverage metrics.  Obviously, this is not a great measure of code coverage.

This script fixes this by letting teams configure what directories should have
tests.

Usage:

1) Create a .golang_coverage_lintrc.json file at the root of your golang repo:
    {
        "skipped paths": [
            # Script assumes that you don't want some common directories like
            # vendor, go-build, etc.
            "relative/path/testlib",
            "another/relative/path/used/in/tests"
        ],
        "skipped regex paths": [
            ".*mocks?$"
        ]
        # Omitting the coverage section makes the script check that all packages
        # have tests without checking the coverage.
        "coverage": {
            "min percentage": 90.0
        }
    }

2) Cause jenkins to execute this script (should happen by default if you use
   `make jenkins`).

3) Your build should now fail diffs that introduce golang packages without
   _test.go files or have less than 90% line coverage.

"""

from __future__ import print_function

import argparse
import logging
import json
import os
import os.path
import re
import sys
import xml.etree.ElementTree

GO_FILE_SUFFIX = '.go'
GO_TEST_FILE_SUFFIX = '_test.go'

COVERAGE_REPORT_FILE = 'coverage.xml'

CONFIG_FILE_NAME = '.golang_coverage_lintrc.json'
CONFIG_KEY_SKIPPED_PATHS = 'skipped paths'
CONFIG_KEY_SKIPPED_REGEX_PATHS = 'skipped regex paths'
CONFIG_KEY_COVERAGE = 'coverage'
CONFIG_KEY_MIN_PERCENTAGE = 'min percentage'

def load_config_file(config_path):
    """Loads the config file and parses the contained JSON."""
    with open(config_path) as config_file:
        try:
            config = json.load(config_file)
        except ValueError as ex:
            logging.error('Could not parse coverage config file %s: %s',
                          config_path, ex)
            return None

    return config

# pylint: disable=too-few-public-methods
class SkippedPathDetecter(object):
    """
    This object tells us whether to skip a path according to the project
    config file.
    """
    _ALWAYS_SKIPPED = (
        '.git',
        '.gen',
        '.tmp',
        'vendor',
        'go-build',
    )

    def __init__(self, path_root, config):
        # Paths relative to the root of the project.
        self._path_root = path_root
        self._skipped_paths = set()
        self._skipped_regex_paths = []
        if CONFIG_KEY_SKIPPED_PATHS in config:
            self._skipped_paths = set(config[CONFIG_KEY_SKIPPED_PATHS])
        if CONFIG_KEY_SKIPPED_REGEX_PATHS in config:
            for pattern in config[CONFIG_KEY_SKIPPED_REGEX_PATHS]:
                self._skipped_regex_paths.append(re.compile(pattern))

    def should_skip(self, path):
        """
        Returns True if we should ignore this path when looking for
        untested packages.
        """
        if not os.path.isdir(path):
            return True
        if os.path.islink(path):
            return True
        rel_path = os.path.relpath(path, self._path_root)
        if rel_path in SkippedPathDetecter._ALWAYS_SKIPPED:
            return True
        if rel_path in self._skipped_paths:
            return True
        for prog in self._skipped_regex_paths:
            if prog.match(rel_path) is not None:
                return True
        return False

def all_directories_have_tests(path_root, config, ignored_file_regex_paths):
    """
    Returns False iff a golang package without tests is found and
    not explicitly whitelisted in the config file.
    """
    skipped_path_detecter = SkippedPathDetecter(path_root, config)
    bad_paths = []
    unvisited_paths = [path_root]
    while unvisited_paths:
        parent_directory_path = unvisited_paths.pop()
        logging.debug('Looking for missing tests in %s', parent_directory_path)
        directory_entries = os.listdir(parent_directory_path)

        # Look for subdirectories we should visit first.
        for entry in directory_entries:
            path = os.path.join(parent_directory_path, entry)
            if skipped_path_detecter.should_skip(path):
                continue
            unvisited_paths.append(path)

        # Check the current directory for missing test files.
        found_go_file = False
        found_test_file = False
        for entry in directory_entries:
            if entry.endswith(GO_FILE_SUFFIX):
                path = os.path.join(parent_directory_path, entry)
                if should_skip_file(path, ignored_file_regex_paths):
                    continue
                found_go_file = True
                found_test_file = (found_test_file or
                                   entry.endswith(GO_TEST_FILE_SUFFIX))

        if found_go_file and not found_test_file:
            bad_paths.append(parent_directory_path)

    if bad_paths:
        logging.error('Found golang packages without _test.go files:')
        for path in bad_paths:
            rel_path = os.path.relpath(path, path_root)
            logging.error('  %s', rel_path)
        return False
    return True

def should_skip_file(path, ignored_file_regex_paths):
    """
    Takes a file path and a list of compiled regular expressions describing
    files to ignore for coverage purposes.
    """
    for reg in ignored_file_regex_paths:
        if reg.match(path) is not None:
            return True
    return False

def coverage_is_sufficient(path_root, config):
    """
    Returns False iff the config file sets a minimum level of coverage and
    this project does not currently meet it.
    """
    if CONFIG_KEY_COVERAGE not in config:
        logging.info('Skipping coverage calculation because config file is '
                     'missing the %s key.', CONFIG_KEY_COVERAGE)
        return True  # Unconfigured is not an error
    if CONFIG_KEY_MIN_PERCENTAGE not in config[CONFIG_KEY_COVERAGE]:
        logging.error('Could not find coverage threshold in %s',
                      CONFIG_FILE_NAME)
        return False  # Misconfigured is an error

    coverage_file_path = os.path.join(path_root, COVERAGE_REPORT_FILE)
    if not os.path.isfile(coverage_file_path):
        logging.error('Could not find coverage report (%s).',
                      coverage_file_path)
        return False

    uncovered_lines = 0
    covered_lines = 0
    root = xml.etree.ElementTree.parse(coverage_file_path).getroot()
    for class_to_check in root.iter('class'):
        for method in class_to_check.iter('method'):
            for line in method.iter('line'):
                if line.get('hits') == '0':
                    uncovered_lines += 1
                else:
                    covered_lines += 1

    total_lines = covered_lines + uncovered_lines
    actual_percent = 100.0 * float(covered_lines) / float(total_lines)
    min_percent = float(config[CONFIG_KEY_COVERAGE][CONFIG_KEY_MIN_PERCENTAGE])
    logging.info('Calculated %d covered lines of %d total lines (%0.2f%%)',
                 covered_lines, total_lines, actual_percent)

    if actual_percent < min_percent:
        logging.error('Expected line coverage of at least %0.2f%% but '
                      'got %0.2f%% (%d/%d)',
                      min_percent, actual_percent, covered_lines, total_lines)
        return False
    return True

def main():
    """Script entry point"""
    args = parse_args()
    path_root = os.getcwd()
    config_path = os.path.join(path_root, CONFIG_FILE_NAME)
    if not os.path.isfile(config_path):
        logging.info('Create %s to enable the golang test coverage linter',
                     CONFIG_FILE_NAME)
        return 0
    config = load_config_file(config_path)
    if config is None:
        return 1

    ignored_file_regex_paths = []
    if args.cover_ignore_srcs is not None:
        for pattern in args.cover_ignore_srcs.strip().split():
            ignored_file_regex_paths.append(re.compile(pattern))

    if not all_directories_have_tests(
            path_root, config,
            ignored_file_regex_paths):
        return 1
    if not coverage_is_sufficient(path_root, config):
        return 1
    return 0

def parse_args():
    """Parse program arguments and set up logging levels."""
    parser = argparse.ArgumentParser(
        description='Check coverage metrics for golang packages.')
    parser.add_argument(
        '-v', '--verbose', dest='verbose', action='store_true',
        default=False, help='Enable verbose mode (default: off)')
    parser.add_argument(
        '-d', '--debug', dest='debug', action='store_true', default=False,
        help='Enable tons of logging (default: off)')
    parser.add_argument(
        '--cover_ignore_srcs', type=str, default=None,
        help='golang filepaths ignored for coverage purposes')
    args = parser.parse_args()
    log_level = logging.WARNING
    if args.debug:
        log_level = logging.DEBUG
    if args.verbose:
        log_level = logging.INFO
    logging.basicConfig(
        format=__file__ + ': %(levelname)s %(message)s',
        level=log_level)
    return args

if __name__ == '__main__':
    sys.exit(main())
