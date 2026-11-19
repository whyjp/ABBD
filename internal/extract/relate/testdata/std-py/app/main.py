import json
import os
import sys

from lib.lib import name

out = json.dumps([os.sep, sys.platform, name()])
