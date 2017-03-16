import requests

from .client import Client as APIClient


class Client:
    def __init__(self, base_uri="http://api.jumpscale.com/{version}"):
        self.api = APIClient(base_uri)
        