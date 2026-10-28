"""Generated protobuf + gRPC stubs (buf out=sdk/py/src/hop_top_nerv/gen).

The buf-emitted ``nerv/v1/event_pb2_grpc.py`` does ``from nerv.v1 import
event_pb2`` -- a top-level ``nerv`` import. Inject this directory onto
``sys.path`` so the gen package resolves without polluting the importer's
own path manipulation. Imports of ``hop_top_nerv.gen.nerv.v1.event_pb2``
continue to work via the relative package layout.
"""

from __future__ import annotations

import os
import sys

_GEN_DIR = os.path.dirname(os.path.abspath(__file__))
if _GEN_DIR not in sys.path:
    sys.path.insert(0, _GEN_DIR)
