#!/usr/bin/env python3

"""
dummy_decryptor.py can be used to mock a secret backend binary
secret handles with "_error" suffix have errors in the response
secret handles with "_ignore" suffix are ignored in the response
"""

from sys import stdin, stdout
from json import loads, dumps


if __name__ == "__main__":
    for line in stdin:
        encrypted = loads(line)
        decrypted = {}
        for handle in encrypted["secrets"]:
            if handle.endswith("_error"):
                decrypted[handle] = {
                    "value": "",
                    "error": "cannot decrypt " + handle,
                }
                continue
            if handle.endswith("_ignore"):
                continue
            decrypted[handle] = {
                "value": "decrypted_" + handle,
            }
        stdout.write(dumps(decrypted))
