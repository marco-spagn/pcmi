from pydantic import BaseModel
from typing import Any, Optional

class MemoryStore(BaseModel):
    path: str
    content: str
    metadata: dict = {}

class MemoryRetrieve(BaseModel):
    path_prefix: str
    query: str
    limit: int = 10