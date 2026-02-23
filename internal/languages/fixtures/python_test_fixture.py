"""
Python Parser Test Fixture
Contains all canonical patterns for parser validation.
Line numbers are predictable for automated testing.
"""

from enum import Enum
from typing import List, Optional, Protocol
from dataclasses import dataclass

# === Pattern 1: Constants ===
MAX_RETRIES = 3
DEFAULT_TIMEOUT = 30
API_VERSION = "1.0.0"
DEBUG_MODE = True

# === Pattern 7: Type Aliases ===
UserId = str
ItemList = List["Item"]


# === Pattern 2: Functions ===
def calculate_total(items: List["Item"]) -> int:
    """Calculate the total value of all items."""
    return sum(item.value for item in items)


def process_items(items: List["Item"], timeout: int = DEFAULT_TIMEOUT) -> bool:
    """Process a list of items with optional timeout."""
    for item in items:
        if not item.is_valid():
            return False
    return True


async def fetch_user(user_id: str) -> Optional["User"]:
    """Asynchronously fetch a user by ID."""
    pass


# === Pattern 5: Enums ===
class Status(Enum):
    """Status enum for items."""
    ACTIVE = 1
    INACTIVE = 2
    PENDING = 3


class Priority(Enum):
    """Priority levels."""
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"


# === Pattern 4: Interfaces (Protocol) ===
class UserRepository(Protocol):
    """Repository interface for user operations."""

    def find_by_id(self, user_id: str) -> Optional["User"]:
        """Find a user by their ID."""
        ...

    def save(self, user: "User") -> bool:
        """Save a user to the repository."""
        ...


class ItemValidator(Protocol):
    """Validator interface for items."""

    def validate(self, item: "Item") -> bool:
        """Validate an item."""
        ...


# === Pattern 3: Classes ===
@dataclass
class Item:
    """Data class representing an item."""
    id: str
    name: str
    value: int
    status: Status = Status.ACTIVE

    def is_valid(self) -> bool:
        """Check if the item is valid."""
        return self.value > 0 and self.status == Status.ACTIVE

    def calculate_discount(self, percentage: float) -> float:
        """Calculate discounted price."""
        return self.value * (1 - percentage / 100)


class UserService:
    """Service class for user operations."""

    def __init__(self, repository: UserRepository):
        """Initialize with a repository."""
        self._repository = repository

    def find_by_id(self, user_id: str) -> Optional["User"]:
        """Find a user by ID."""
        return self._repository.find_by_id(user_id)

    async def create_user(self, name: str, email: str) -> "User":
        """Create a new user."""
        pass

    def update_user(self, user_id: str, data: dict) -> bool:
        """Update an existing user."""
        pass


class BaseEntity:
    """Base class for all entities."""

    id: str
    created_at: str

    def get_id(self) -> str:
        """Return the entity ID."""
        return self.id


class User(BaseEntity):
    """User entity extending BaseEntity."""

    name: str
    email: str
    status: Status

    def __init__(self, id: str, name: str, email: str):
        """Initialize user."""
        self.id = id
        self.name = name
        self.email = email
        self.status = Status.ACTIVE

    def deactivate(self) -> None:
        """Deactivate the user."""
        self.status = Status.INACTIVE

    def is_active(self) -> bool:
        """Check if user is active."""
        return self.status == Status.ACTIVE


# === Private/Internal symbols (should have Exported=false) ===
_INTERNAL_CONSTANT = "internal"


def _private_helper(data: str) -> str:
    """Private helper function."""
    return data.strip()


class _InternalCache:
    """Internal cache implementation."""

    def __init__(self):
        self._data = {}

    def get(self, key: str):
        """Get value from cache."""
        return self._data.get(key)
