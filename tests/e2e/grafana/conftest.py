"""Shared fixtures for Grafana Playwright e2e tests."""

import os
import pytest
from playwright.sync_api import Page, Browser, BrowserContext

GRAFANA_URL = os.environ.get("GRAFANA_URL", "http://localhost:3000")
GRAFANA_USER = os.environ.get("GRAFANA_USER", "admin")
GRAFANA_PASS = os.environ.get("GRAFANA_PASS", "admin")

# Dashboard UIDs
DASHBOARDS = {
    "overview": "mdemg-overview",
    "neo4j": "mdemg-neo4j",
    "rsic": "mdemg-rsic",
    "j17": "mdemg-j17",
    "jiminy": "mdemg-jiminy",
    "graph-topology": "mdemg-graph-topology",
    "ft-training": "mdemg-ft-training",
}


@pytest.fixture(scope="session")
def grafana_url():
    return GRAFANA_URL


@pytest.fixture(scope="session")
def browser_context_args(browser_context_args):
    return {
        **browser_context_args,
        "viewport": {"width": 1920, "height": 1080},
        "storage_state": None,
    }


@pytest.fixture(scope="session")
def _grafana_storage_state(browser: Browser, grafana_url):
    """Log in once via the Grafana login form and save session cookies."""
    context = browser.new_context(viewport={"width": 1920, "height": 1080})
    page = context.new_page()
    page.goto(f"{grafana_url}/login", wait_until="networkidle")

    # Fill the login form (Grafana 10.x aria labels)
    page.get_by_label("Username input field").fill(GRAFANA_USER)
    page.get_by_label("Password input field").fill(GRAFANA_PASS)
    page.get_by_label("Login button").click()
    page.wait_for_timeout(2000)

    # Grafana may prompt to change default password — click "Skip" if present
    skip_btn = page.get_by_role("button", name="Skip")
    if skip_btn.is_visible():
        skip_btn.click()

    # Wait for redirect away from /login
    page.wait_for_function("() => !window.location.pathname.includes('/login')", timeout=10000)
    page.wait_for_load_state("networkidle")

    # Save cookies/storage for reuse
    state = context.storage_state()
    context.close()
    return state


@pytest.fixture()
def authenticated_page(page: Page, context: BrowserContext, _grafana_storage_state, grafana_url: str):
    """Provide a page with Grafana session cookies already set."""
    context.add_cookies(_grafana_storage_state["cookies"])
    return page


def dashboard_url(grafana_url: str, uid: str, **params) -> str:
    """Build a Grafana dashboard URL with optional var- query params."""
    base = f"{grafana_url}/d/{uid}/{uid}"
    if params:
        qs = "&".join(f"var-{k}={v}" for k, v in params.items())
        return f"{base}?{qs}"
    return base
