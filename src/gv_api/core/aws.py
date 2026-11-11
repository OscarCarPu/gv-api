import os
from typing import Any

import boto3
from botocore.config import Config


def client(service: str) -> Any:
    """A boto3 client configured like the deploy configures the service: AWS_REGION (which
    boto3 does not read by itself) and, with a custom endpoint such as the local fake AWS,
    path-style S3 addresses (it has no bucket.host name to resolve)."""
    config = Config(region_name=os.environ.get("AWS_REGION") or None)
    if os.environ.get("AWS_ENDPOINT_URL"):
        config = config.merge(Config(s3={"addressing_style": "path"}))
    return boto3.client(service, config=config)  # type: ignore[call-overload]
