#!/usr/bin/env python3
"""Chase teams for unfinished QA cards for a given operator release version.

Usage:
    python hack/release/chase_qa_cards.py --version 1.9.0
    python hack/release/chase_qa_cards.py --version 1.9.0 --dry-run

Required environment variables:
    ATLASSIAN_USERNAME          Jira username (email)
    ATLASSIAN_PASSWORD          Jira API token
    SLACK_DATADOG_AGENT_BOT_TOKEN  Slack bot token (not required for --dry-run)
"""
import argparse
import os
import pathlib
import sys
from collections import defaultdict

import yaml

SCRIPT_DIR = pathlib.Path(__file__).parent
DEFAULT_SLACK_CHANNEL = "#container-platform"
DEFAULT_JIRA_PROJECT = "CONTP"


def _load_map(filename, default_placeholder, default_value, channel_type='notification'):
    result = {}
    with (SCRIPT_DIR / filename).open(encoding='utf-8') as f:
        for key, value in yaml.safe_load(f).items():
            if isinstance(value, dict) and channel_type in value:
                value = value[channel_type]
            if isinstance(value, dict):
                value = value.get('name', '')
            result[key] = default_value if value == default_placeholder else value
    return result


def list_not_closed_qa_cards(version):
    from atlassian import Jira

    jira = Jira(
        url="https://datadoghq.atlassian.net",
        username=os.environ['ATLASSIAN_USERNAME'],
        password=os.environ['ATLASSIAN_PASSWORD'],
        cloud=True,
    )
    jql = (
        f'labels in (ddqa) and labels not in (test_ignore) and labels in ({version}-qa) '
        'and status not in ((Done, DONE, "Won\'t Fix", "WON\'T FIX", "In Progress", '
        '"Testing/Review", "In review", "✅ Done", "won\'t do", Duplicate, Closed, '
        '"NOT DOING", not-do, canceled, QA)) order by created desc'
    )
    return jira.enhanced_jql(jql)['issues']


def chase_for_qa_cards(version, dry_run=False):
    cards = list_not_closed_qa_cards(version)
    if not cards:
        print(f"No QA cards to chase for {version} — all done!")
        return

    grouped = defaultdict(list)
    for card in cards:
        grouped[card['fields']['project']['key']].append(card)

    slack_map = _load_map('github_slack_map.yaml', 'DEFAULT_SLACK_CHANNEL', DEFAULT_SLACK_CHANNEL)
    jira_map = _load_map('github_jira_map.yaml', 'DEFAULT_JIRA_PROJECT', DEFAULT_JIRA_PROJECT)
    jira_to_team = {project: team for team, project in jira_map.items()}

    print(f"Found {len(cards)} unfinished QA card(s) for {version}:")

    if not dry_run:
        from slack_sdk import WebClient
        client = WebClient(os.environ['SLACK_DATADOG_AGENT_BOT_TOKEN'])

    for project, project_cards in grouped.items():
        team = jira_to_team.get(project)
        if not team:
            print(
                f"  WARNING: no team found for Jira project {project}, "
                f"skipping {[c['key'] for c in project_cards]}",
                file=sys.stderr,
            )
            continue

        channel = slack_map.get(team)
        if not channel:
            print(f"  WARNING: no Slack channel for team {team}, skipping", file=sys.stderr)
            continue

        card_links = ', '.join(
            f"<https://datadoghq.atlassian.net/browse/{c['key']}|{c['key']}>"
            for c in project_cards
        )
        message = (
            f"Hello :wave:\nCould you please update the QA cards {card_links} "
            f"for the {version} release?\nThanks in advance"
        )

        print(f"  -> {channel} ({team}): {[c['key'] for c in project_cards]}")
        if not dry_run:
            client.chat_postMessage(channel=channel, text=message)

    if dry_run:
        print("\n(dry-run: no Slack messages sent)")


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Chase teams for unfinished operator QA cards')
    parser.add_argument('--version', required=True, help='Release version, e.g. 1.9.0')
    parser.add_argument('--dry-run', action='store_true', help='Print planned messages without posting to Slack')
    args = parser.parse_args()
    chase_for_qa_cards(args.version, dry_run=args.dry_run)
