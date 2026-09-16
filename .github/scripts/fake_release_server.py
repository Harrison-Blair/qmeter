#!/usr/bin/env python3
"""A tiny stand-in for GitHub's release endpoints, used by install_test.sh.

Serves ``--root`` as a static tree, so ``/releases/download/<tag>/<asset>`` and
``/releases/download/<tag>/checksums.txt`` come straight off disk, and answers
``/releases/latest`` with the 302 redirect to ``/releases/tag/<tag>`` that the
installers read the latest tag out of. With ``--latest`` empty it answers 404
instead, which is what GitHub does for a repository that has no release yet.

Binds 127.0.0.1 on an ephemeral port and prints that port on stdout, one line,
before serving.
"""

import argparse
import functools
import http.server
import socketserver
import sys


class Handler(http.server.SimpleHTTPRequestHandler):
    latest = ""

    def _latest_redirect(self):
        """Answer /releases/latest; return True when the path was handled."""
        if self.path.rstrip("/") != "/releases/latest":
            return False
        if not self.latest:
            self.send_error(404, "no releases")
            return True
        self.send_response(302)
        self.send_header("Location", "/releases/tag/%s" % self.latest)
        self.send_header("Content-Length", "0")
        self.end_headers()
        return True

    def do_GET(self):
        if not self._latest_redirect():
            super().do_GET()

    def do_HEAD(self):
        if not self._latest_redirect():
            super().do_HEAD()

    def log_message(self, fmt, *args):
        sys.stderr.write("fake-release-server: " + (fmt % args) + "\n")


class Server(socketserver.TCPServer):
    allow_reuse_address = True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", required=True, help="directory to serve")
    parser.add_argument("--latest", default="", help="tag /releases/latest redirects to")
    args = parser.parse_args()

    Handler.latest = args.latest
    handler = functools.partial(Handler, directory=args.root)

    with Server(("127.0.0.1", 0), handler) as httpd:
        print(httpd.server_address[1], flush=True)
        httpd.serve_forever()


if __name__ == "__main__":
    main()
