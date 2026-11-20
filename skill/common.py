"""Settings shared by render.py, server.py and autostart.py.

Everything is resolved from the environment and ~/.whatnow/config.json, never from a fixed path:

  home          $WHATNOW_HOME, else ~/.whatnow
  featuresDir   $FEATURES_DIR, else "featuresDir" in <home>/config.json, else <home>/features
  port          "port" in <home>/config.json, else 7777
  aliases       "aliases" in <home>/config.json: {repo folder name: project name used in feature.json}
"""
import json, os

DEFAULT_PORT = 7777


def home():
    return os.environ.get('WHATNOW_HOME') or os.path.expanduser('~/.whatnow')


def config():
    try:
        with open(os.path.join(home(), 'config.json'), encoding='utf-8') as fh:
            data = json.load(fh)
        return data if isinstance(data, dict) else {}
    except (OSError, ValueError):
        return {}


def features_dir():
    raw = os.environ.get('FEATURES_DIR') or config().get('featuresDir') or os.path.join(home(), 'features')
    return os.path.abspath(os.path.expanduser(raw))


def port():
    try:
        return int(config().get('port', DEFAULT_PORT))
    except (TypeError, ValueError):
        return DEFAULT_PORT
