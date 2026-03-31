"""Shared fixtures for MDEMG browser dashboard Playwright e2e tests."""

import os
import pytest
from playwright.sync_api import Page, Browser, BrowserContext

MDEMG_URL = os.environ.get("MDEMG_URL", "http://localhost:9999")


@pytest.fixture(scope="session")
def mdemg_url():
    return MDEMG_URL


@pytest.fixture(scope="session")
def browser_context_args(browser_context_args):
    return {
        **browser_context_args,
        "viewport": {"width": 1280, "height": 900},
    }


@pytest.fixture()
def ui_page(page: Page, mdemg_url: str):
    """Navigate to the browser dashboard and wait for it to load."""
    response = page.goto(f"{mdemg_url}/ui/", wait_until="networkidle")
    assert response is not None, "No response from /ui/"
    assert response.status == 200, f"Expected 200, got {response.status}"
    # Wait for the JS module to initialize and render the default tab
    page.wait_for_selector("#content", state="attached", timeout=10000)
    page.wait_for_timeout(1000)  # Allow initial polling cycle
    return page
