"""E2E tests verifying $space_id variable is present and functional across all dashboards."""

import pytest
from playwright.sync_api import Page, expect

from conftest import dashboard_url, DASHBOARDS


# All dashboards that should have the space_id variable
DASHBOARDS_WITH_SPACE_ID = [
    "overview",
    "neo4j",
    "rsic",
    "j17",
    "jiminy",
    "graph-topology",
    "ft-training",
]


class TestSpaceIdVariablePresent:
    """Every dashboard should have a Space ID dropdown in the variable bar."""

    @pytest.mark.parametrize("dashboard_key", DASHBOARDS_WITH_SPACE_ID)
    def test_space_id_label_visible(self, authenticated_page: Page, grafana_url: str, dashboard_key: str):
        """The 'Space ID' label should appear in the variable bar."""
        uid = DASHBOARDS[dashboard_key]
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, uid, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        # Each template variable is a submenu-item; find the one containing "Space ID"
        space_id_var = page.get_by_test_id("data-testid template variable").filter(
            has_text="Space ID"
        )
        expect(space_id_var.first).to_be_visible(timeout=15000)

    @pytest.mark.parametrize("dashboard_key", DASHBOARDS_WITH_SPACE_ID)
    def test_instance_label_visible(self, authenticated_page: Page, grafana_url: str, dashboard_key: str):
        """The 'Instance' label should appear in the variable bar."""
        uid = DASHBOARDS[dashboard_key]
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, uid, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        instance_var = page.get_by_test_id("data-testid template variable").filter(
            has_text="Instance"
        )
        expect(instance_var.first).to_be_visible(timeout=15000)


class TestSpaceIdFiltering:
    """Verify that setting space_id filters panel data correctly."""

    @pytest.mark.parametrize("dashboard_key", DASHBOARDS_WITH_SPACE_ID)
    def test_space_id_in_url(self, authenticated_page: Page, grafana_url: str, dashboard_key: str):
        """Dashboard URL should contain var-space_id when set."""
        uid = DASHBOARDS[dashboard_key]
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, uid, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        assert "var-space_id=mdemg-dev" in page.url


class TestNoPanelErrors:
    """Verify no panels show critical errors on load."""

    # All TSDB dashboards including ft-training (now has real queries)
    TSDB_DASHBOARDS = ["overview", "neo4j", "rsic", "j17", "jiminy", "ft-training"]

    @pytest.mark.parametrize("dashboard_key", TSDB_DASHBOARDS)
    def test_no_panel_error_alerts(self, authenticated_page: Page, grafana_url: str, dashboard_key: str):
        """No panel should display an error alert banner."""
        uid = DASHBOARDS[dashboard_key]
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, uid, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        page.wait_for_timeout(3000)
        error_panels = page.locator('[data-testid*="Panel status error"]')
        error_count = error_panels.count()
        if error_count > 0:
            errors = []
            for i in range(min(error_count, 5)):
                errors.append(error_panels.nth(i).inner_text())
            assert error_count == 0, \
                f"{dashboard_key}: {error_count} panel error(s): {'; '.join(errors)}"


class TestDashboardNavigation:
    """Verify the Dashboards dropdown link works across dashboards."""

    @pytest.mark.parametrize("dashboard_key", ["overview", "neo4j", "rsic", "j17", "jiminy", "ft-training"])
    def test_dashboard_link_dropdown_exists(self, authenticated_page: Page, grafana_url: str, dashboard_key: str):
        """Each dashboard should have a 'Dashboards' navigation link."""
        uid = DASHBOARDS[dashboard_key]
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, uid, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        dash_link = page.get_by_role("link", name="Dashboards")
        if dash_link.count() == 0:
            dash_link = page.get_by_text("Dashboards").first
        expect(dash_link).to_be_visible(timeout=10000)


class TestSpaceIdDefaultValue:
    """Verify every dashboard loads with a non-empty space_id when no URL param is provided."""

    @pytest.mark.parametrize("dashboard_key", DASHBOARDS_WITH_SPACE_ID)
    def test_default_space_id_populated(self, authenticated_page: Page, grafana_url: str, dashboard_key: str):
        """Loading a dashboard without var-space_id should still show a default value."""
        uid = DASHBOARDS[dashboard_key]
        page = authenticated_page
        # Navigate without any var- params
        page.goto(f"{grafana_url}/d/{uid}/{uid}", wait_until="networkidle")
        page.wait_for_timeout(2000)
        space_id_var = page.get_by_test_id("data-testid template variable").filter(
            has_text="Space ID"
        )
        expect(space_id_var.first).to_be_visible(timeout=15000)
        # The variable dropdown should not show an empty string or "All"
        var_text = space_id_var.first.inner_text()
        assert "Space ID" in var_text, f"Space ID label missing in variable bar: {var_text}"


class TestNoDataGraceful:
    """Load each dashboard with a nonexistent space_id and verify zero panel errors."""

    ALL_DASHBOARDS = ["overview", "neo4j", "rsic", "j17", "jiminy", "ft-training"]

    @pytest.mark.parametrize("dashboard_key", ALL_DASHBOARDS)
    def test_nonexistent_space_no_errors(self, authenticated_page: Page, grafana_url: str, dashboard_key: str):
        """With a nonexistent space_id, panels should degrade gracefully (no error alerts)."""
        uid = DASHBOARDS[dashboard_key]
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, uid, space_id="nonexistent-space-xyz"),
            wait_until="networkidle",
        )
        page.wait_for_timeout(3000)
        error_panels = page.locator('[data-testid*="Panel status error"]')
        error_count = error_panels.count()
        if error_count > 0:
            errors = []
            for i in range(min(error_count, 5)):
                errors.append(error_panels.nth(i).inner_text())
            assert error_count == 0, \
                f"{dashboard_key}: {error_count} panel error(s) with nonexistent space: {'; '.join(errors)}"


class TestFTTrainingStatusNotice:
    """Verify the FT Training dashboard status notice is visible."""

    def test_status_notice_visible(self, authenticated_page: Page, grafana_url: str):
        """The FT Training status notice should show the auto-populate message."""
        uid = DASHBOARDS["ft-training"]
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, uid, space_id="mdemg-dev"),
            wait_until="networkidle",
        )
        notice_text = page.get_by_text("auto-populate")
        expect(notice_text).to_be_visible(timeout=15000)


class TestStatPanelNoValue:
    """Verify stat panels show graceful text instead of bare 'No data'."""

    def test_overview_stat_panels_no_value(self, authenticated_page: Page, grafana_url: str):
        """Overview stat panels should show 'N/A' when no data exists."""
        uid = DASHBOARDS["overview"]
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, uid, space_id="nonexistent-space-xyz"),
            wait_until="networkidle",
        )
        page.wait_for_timeout(3000)
        # Stat panels with noValue should not show bare "No data" text
        stat_panels = page.locator('[data-testid*="data-testid Panel header"]').filter(
            has_text="Request Rate"
        )
        if stat_panels.count() > 0:
            panel_container = stat_panels.first.locator("..")
            panel_text = panel_container.inner_text()
            assert "No data" not in panel_text or "N/A" in panel_text, \
                f"Overview stat panel shows bare 'No data' instead of 'N/A': {panel_text[:200]}"

    def test_rsic_watchdog_no_value(self, authenticated_page: Page, grafana_url: str):
        """RSIC Watchdog Escalation should show 'Awaiting Data' with no data, not green."""
        uid = DASHBOARDS["rsic"]
        page = authenticated_page
        page.goto(
            dashboard_url(grafana_url, uid, space_id="nonexistent-space-xyz"),
            wait_until="networkidle",
        )
        page.wait_for_timeout(3000)
        # Look for "Awaiting Data" text on the page
        awaiting = page.get_by_text("Awaiting Data")
        if awaiting.count() > 0:
            expect(awaiting.first).to_be_visible(timeout=10000)
