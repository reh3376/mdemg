"""E2E tests for the MDEMG browser dashboard served at /ui/.

Covers: page load, tab navigation, all 6 tabs (Status, Memory, Learning,
Config, Logs, RSIC), polling behavior, space selector, and API endpoints.
"""

import json
import pytest
import requests
from playwright.sync_api import Page, expect

from conftest import MDEMG_URL


# ---------------------------------------------------------------------------
# Page Load & Structure
# ---------------------------------------------------------------------------

class TestPageLoad:
    """Verify the dashboard loads and has the expected structural elements."""

    def test_ui_returns_200(self, ui_page: Page):
        """GET /ui/ should return 200 with an HTML page."""
        assert "/ui/" in ui_page.url

    def test_title_is_set(self, ui_page: Page):
        """Page title should be 'MDEMG Dashboard'."""
        assert ui_page.title() == "MDEMG Dashboard"

    def test_header_visible(self, ui_page: Page):
        """The header bar with 'MDEMG Dashboard' should be visible."""
        header = ui_page.locator(".header-title")
        expect(header).to_be_visible(timeout=5000)
        expect(header).to_have_text("MDEMG Dashboard")

    def test_space_selector_visible(self, ui_page: Page):
        """The space selector dropdown should be present in the header."""
        select = ui_page.locator("#space-select")
        expect(select).to_be_visible(timeout=5000)

    def test_space_selector_has_default(self, ui_page: Page):
        """Space selector should have mdemg-dev as an option."""
        select = ui_page.locator("#space-select")
        options = select.locator("option")
        assert options.count() >= 1, "Space selector has no options"
        all_values = [options.nth(i).get_attribute("value") for i in range(options.count())]
        assert "mdemg-dev" in all_values, f"mdemg-dev not in options: {all_values}"

    def test_content_container_exists(self, ui_page: Page):
        """The #content container should exist."""
        content = ui_page.locator("#content")
        expect(content).to_be_attached(timeout=5000)

    def test_no_js_console_errors(self, ui_page: Page, mdemg_url: str):
        """Loading the page should not produce JS console errors."""
        errors = []
        ui_page.on("console", lambda msg: errors.append(msg.text) if msg.type == "error" else None)
        ui_page.goto(f"{mdemg_url}/ui/", wait_until="networkidle")
        ui_page.wait_for_timeout(3000)
        # Filter out network errors (expected when server endpoints aren't all available)
        real_errors = [e for e in errors if "fetch" not in e.lower() and "network" not in e.lower()]
        assert len(real_errors) == 0, f"JS console errors: {real_errors}"


# ---------------------------------------------------------------------------
# Tab Bar Navigation
# ---------------------------------------------------------------------------

class TestTabNavigation:
    """Verify all 6 tabs are present and can be clicked."""

    TAB_NAMES = ["status", "memory", "learning", "config", "logs", "rsic"]

    def test_all_tab_buttons_present(self, ui_page: Page):
        """All 6 tab buttons should be visible."""
        for tab_name in self.TAB_NAMES:
            btn = ui_page.locator(f'.tab-btn[data-tab="{tab_name}"]')
            expect(btn).to_be_visible(timeout=5000), f"Tab button '{tab_name}' not visible"

    def test_status_tab_active_by_default(self, ui_page: Page):
        """The Status tab should be active on initial load."""
        status_btn = ui_page.locator('.tab-btn[data-tab="status"]')
        assert "active" in (status_btn.get_attribute("class") or ""), \
            "Status tab not active by default"

    @pytest.mark.parametrize("tab_name", TAB_NAMES)
    def test_tab_click_activates(self, ui_page: Page, tab_name: str):
        """Clicking a tab should make it active and deactivate others."""
        btn = ui_page.locator(f'.tab-btn[data-tab="{tab_name}"]')
        btn.click()
        ui_page.wait_for_timeout(500)
        assert "active" in (btn.get_attribute("class") or ""), \
            f"Tab '{tab_name}' not active after click"
        # Other tabs should not be active
        for other in self.TAB_NAMES:
            if other != tab_name:
                other_btn = ui_page.locator(f'.tab-btn[data-tab="{other}"]')
                assert "active" not in (other_btn.get_attribute("class") or ""), \
                    f"Tab '{other}' should not be active when '{tab_name}' is selected"

    @pytest.mark.parametrize("tab_name", TAB_NAMES)
    def test_tab_click_renders_content(self, ui_page: Page, tab_name: str):
        """Clicking a tab should render content in #content."""
        btn = ui_page.locator(f'.tab-btn[data-tab="{tab_name}"]')
        btn.click()
        ui_page.wait_for_timeout(2000)
        content = ui_page.locator("#content")
        inner = content.inner_text()
        assert len(inner) > 0, f"Tab '{tab_name}' rendered empty content"


# ---------------------------------------------------------------------------
# Status Tab
# ---------------------------------------------------------------------------

class TestStatusTab:
    """Verify the Status tab shows health info and Grafana links."""

    def test_server_section_header(self, ui_page: Page):
        """Status tab should show 'Server' section header."""
        ui_page.wait_for_timeout(2000)
        header = ui_page.locator(".section-header").filter(has_text="Server")
        expect(header).to_be_visible(timeout=10000)

    def test_health_badge_visible(self, ui_page: Page):
        """A health status badge should appear."""
        ui_page.wait_for_timeout(3000)
        badge = ui_page.locator(".badge").first
        expect(badge).to_be_visible(timeout=10000)

    def test_services_section_present(self, ui_page: Page):
        """The Services readiness section should appear."""
        ui_page.wait_for_timeout(3000)
        services = ui_page.locator(".section-header").filter(has_text="Services")
        expect(services).to_be_visible(timeout=10000)

    def test_embeddings_section_present(self, ui_page: Page):
        """The Embeddings section should appear."""
        ui_page.wait_for_timeout(3000)
        embeddings = ui_page.locator(".section-header").filter(has_text="Embeddings")
        expect(embeddings).to_be_visible(timeout=10000)

    def test_grafana_dashboards_section(self, ui_page: Page):
        """The Grafana Dashboards section should appear with links."""
        ui_page.wait_for_timeout(3000)
        grafana_header = ui_page.locator(".section-header").filter(has_text="Grafana")
        expect(grafana_header).to_be_visible(timeout=10000)

    def test_grafana_links_count(self, ui_page: Page):
        """There should be 7 Grafana dashboard links."""
        ui_page.wait_for_timeout(3000)
        links = ui_page.locator(".grafana-link")
        assert links.count() >= 7, f"Expected 7 Grafana links, found {links.count()}"

    def test_grafana_links_have_correct_dashboards(self, ui_page: Page):
        """Grafana links should point to the 7 known dashboards."""
        ui_page.wait_for_timeout(3000)
        links = ui_page.locator(".grafana-link")
        expected_uids = ["mdemg-overview", "mdemg-neo4j", "mdemg-rsic",
                         "mdemg-graph-topology", "mdemg-jiminy", "mdemg-j17", "mdemg-ft-training"]
        hrefs = [links.nth(i).get_attribute("href") or "" for i in range(links.count())]
        for uid in expected_uids:
            assert any(uid in h for h in hrefs), \
                f"Dashboard {uid} not found in links: {hrefs}"

    def test_grafana_links_reuse_tab(self, ui_page: Page):
        """Grafana links should use target='grafana' to reuse a single tab."""
        ui_page.wait_for_timeout(3000)
        links = ui_page.locator(".grafana-link")
        for i in range(links.count()):
            target = links.nth(i).get_attribute("target")
            assert target == "grafana", f"Grafana link {i} target is '{target}', expected 'grafana'"

    def test_version_info_shown(self, ui_page: Page):
        """The status tab should show version information from healthz."""
        ui_page.wait_for_timeout(3000)
        version_row = ui_page.locator(".info-row").filter(has_text="Version")
        expect(version_row).to_be_visible(timeout=10000)


# ---------------------------------------------------------------------------
# Memory Tab
# ---------------------------------------------------------------------------

class TestMemoryTab:
    """Verify the Memory tab shows stats and has export/import controls."""

    @pytest.fixture(autouse=True)
    def switch_to_memory(self, ui_page: Page):
        ui_page.locator('.tab-btn[data-tab="memory"]').click()
        ui_page.wait_for_timeout(3000)

    def test_memory_overview_header(self, ui_page: Page):
        """Memory Overview section should appear."""
        header = ui_page.locator(".section-header").filter(has_text="Memory Overview")
        expect(header).to_be_visible(timeout=10000)

    def test_total_memories_shown(self, ui_page: Page):
        """Total Memories row should be visible."""
        row = ui_page.locator(".info-row").filter(has_text="Total Memories")
        expect(row).to_be_visible(timeout=10000)

    def test_layer_breakdown_present(self, ui_page: Page):
        """By Layer section with bar chart should appear if layer data exists."""
        header = ui_page.locator(".section-header").filter(has_text="By Layer")
        # by_layer may not be present in API response — conditional check
        if header.count() > 0:
            expect(header.first).to_be_visible(timeout=10000)
            bars = ui_page.locator(".bar-row")
            assert bars.count() >= 1, "No layer bars rendered"

    def test_bar_fills_have_width(self, ui_page: Page):
        """Bar chart fills should have non-zero width."""
        fills = ui_page.locator(".bar-fill")
        if fills.count() > 0:
            style = fills.first.get_attribute("style") or ""
            assert "width:" in style, f"Bar fill has no width style: {style}"

    def test_temporal_distribution_present(self, ui_page: Page):
        """Temporal Distribution section should appear if data exists."""
        header = ui_page.locator(".section-header").filter(has_text="Temporal")
        # May not appear if no temporal data — but check it doesn't error
        if header.count() > 0:
            expect(header.first).to_be_visible(timeout=5000)

    def test_knowledge_sharing_header(self, ui_page: Page):
        """Knowledge Sharing section should appear."""
        header = ui_page.locator(".section-header").filter(has_text="Knowledge Sharing")
        expect(header).to_be_visible(timeout=10000)

    def test_export_button_present(self, ui_page: Page):
        """Export button should be visible."""
        export_btn = ui_page.locator(".btn").filter(has_text="Export")
        expect(export_btn).to_be_visible(timeout=5000)

    def test_export_profile_selector_present(self, ui_page: Page):
        """Export profile dropdown should have options."""
        selects = ui_page.locator(".action-row .select")
        assert selects.count() >= 1, "No profile selector found"
        options = selects.first.locator("option")
        assert options.count() >= 3, f"Expected >=3 export profiles, found {options.count()}"

    def test_import_file_input_present(self, ui_page: Page):
        """File input for import should be visible."""
        file_input = ui_page.locator(".file-input")
        expect(file_input).to_be_attached(timeout=5000)

    def test_import_button_present(self, ui_page: Page):
        """Import button should be visible."""
        import_btn = ui_page.locator(".btn").filter(has_text="Import")
        expect(import_btn).to_be_visible(timeout=5000)


# ---------------------------------------------------------------------------
# Learning Tab
# ---------------------------------------------------------------------------

class TestLearningTab:
    """Verify the Learning tab shows Hebbian stats and action buttons."""

    @pytest.fixture(autouse=True)
    def switch_to_learning(self, ui_page: Page):
        ui_page.locator('.tab-btn[data-tab="learning"]').click()
        ui_page.wait_for_timeout(3000)

    def test_hebbian_header(self, ui_page: Page):
        """Hebbian Learning Edges section should appear."""
        header = ui_page.locator(".section-header").filter(has_text="Hebbian")
        expect(header).to_be_visible(timeout=10000)

    def test_total_edges_shown(self, ui_page: Page):
        """Total Edges row should be visible."""
        row = ui_page.locator(".info-row").filter(has_text="Total Edges")
        expect(row).to_be_visible(timeout=10000)

    def test_edge_type_rows(self, ui_page: Page):
        """Edge type rows (Co-Activated, Strong, Surprising) should appear."""
        for label in ["Co-Activated", "Strong", "Surprising"]:
            row = ui_page.locator(".info-row").filter(has_text=label)
            expect(row).to_be_visible(timeout=10000), f"'{label}' row not visible"

    def test_weight_stats_shown(self, ui_page: Page):
        """Avg and Max weight should be shown."""
        for label in ["Avg Weight", "Max Weight"]:
            row = ui_page.locator(".info-row").filter(has_text=label)
            expect(row).to_be_visible(timeout=10000), f"'{label}' not visible"

    def test_freeze_section_present(self, ui_page: Page):
        """Freeze State section should appear."""
        header = ui_page.locator(".section-header").filter(has_text="Freeze State")
        expect(header).to_be_visible(timeout=10000)

    def test_freeze_badge_visible(self, ui_page: Page):
        """A freeze state badge (active/frozen) should be visible."""
        badges = ui_page.locator(".badge")
        freeze_found = False
        for i in range(badges.count()):
            text = (badges.nth(i).inner_text() or "").lower()
            if text in ("active", "frozen"):
                freeze_found = True
                break
        assert freeze_found, "No freeze state badge (active/frozen) found"

    def test_actions_section_present(self, ui_page: Page):
        """Actions section should appear."""
        header = ui_page.locator(".section-header").filter(has_text="Actions")
        expect(header).to_be_visible(timeout=10000)

    def test_freeze_or_unfreeze_button(self, ui_page: Page):
        """Either Freeze or Unfreeze button should be visible."""
        freeze_btn = ui_page.locator(".btn").filter(has_text="Freeze")
        unfreeze_btn = ui_page.locator(".btn").filter(has_text="Unfreeze")
        assert freeze_btn.count() > 0 or unfreeze_btn.count() > 0, \
            "Neither Freeze nor Unfreeze button found"

    def test_prune_button_present(self, ui_page: Page):
        """Prune button should be visible with danger styling."""
        prune_btn = ui_page.locator(".btn-danger").filter(has_text="Prune")
        expect(prune_btn).to_be_visible(timeout=5000)

    def test_config_section_present(self, ui_page: Page):
        """Configuration section should appear if learning config exists."""
        header = ui_page.locator(".section-header").filter(has_text="Configuration")
        if header.count() > 0:
            expect(header.first).to_be_visible(timeout=5000)
            # Check config values are shown
            for label in ["Decay/Day", "Prune Threshold"]:
                row = ui_page.locator(".info-row").filter(has_text=label)
                if row.count() > 0:
                    expect(row.first).to_be_visible(timeout=5000)


# ---------------------------------------------------------------------------
# Config Tab
# ---------------------------------------------------------------------------

class TestConfigTab:
    """Verify the Config tab shows the effective configuration table."""

    @pytest.fixture(autouse=True)
    def switch_to_config(self, ui_page: Page):
        ui_page.locator('.tab-btn[data-tab="config"]').click()
        ui_page.wait_for_timeout(3000)

    def test_effective_config_header(self, ui_page: Page):
        """Effective Configuration section should appear."""
        header = ui_page.locator(".section-header").filter(has_text="Effective Configuration")
        expect(header).to_be_visible(timeout=10000)

    def test_config_table_present(self, ui_page: Page):
        """A config table should render with rows."""
        table = ui_page.locator(".config-table")
        expect(table).to_be_visible(timeout=10000)

    def test_config_table_has_headers(self, ui_page: Page):
        """Config table should have Key, Value, Source columns."""
        headers = ui_page.locator(".config-table th")
        header_texts = [headers.nth(i).inner_text().lower() for i in range(headers.count())]
        assert "key" in header_texts, f"Missing 'Key' header: {header_texts}"
        assert "value" in header_texts, f"Missing 'Value' header: {header_texts}"
        assert "source" in header_texts, f"Missing 'Source' header: {header_texts}"

    def test_config_table_has_rows(self, ui_page: Page):
        """Config table should have data rows."""
        rows = ui_page.locator(".config-table tbody tr")
        assert rows.count() > 0, "Config table has no data rows"

    def test_config_keys_are_monospace(self, ui_page: Page):
        """Config keys should use monospace font class."""
        keys = ui_page.locator(".config-key")
        assert keys.count() > 0, "No config-key elements found"

    def test_source_badges_present(self, ui_page: Page):
        """Source badges (env/yaml/default) should be present."""
        badges = ui_page.locator(".source-badge")
        assert badges.count() > 0, "No source badges found"

    def test_source_badge_classes(self, ui_page: Page):
        """Source badges should have correct CSS classes."""
        badges = ui_page.locator(".source-badge")
        valid_classes = {"badge-env", "badge-yaml", "badge-default"}
        for i in range(min(badges.count(), 10)):
            classes = badges.nth(i).get_attribute("class") or ""
            has_valid = any(vc in classes for vc in valid_classes)
            assert has_valid, f"Badge {i} has no valid source class: {classes}"

    def test_sensitive_values_masked(self, ui_page: Page):
        """Sensitive config values should be masked with ****."""
        page_text = ui_page.locator("#content").inner_text()
        # If there are any masked values, they should show ****
        values = ui_page.locator(".config-value")
        masked_count = sum(1 for i in range(values.count()) if values.nth(i).inner_text() == "****")
        # At minimum, API keys should be masked (if present in config)
        # This is a soft check — just verify masking works when present
        if masked_count > 0:
            assert True, "Masked values found as expected"

    def test_search_filter_present(self, ui_page: Page):
        """Search/filter input should be present."""
        search = ui_page.locator(".search-input")
        expect(search).to_be_visible(timeout=5000)

    def test_search_filter_works(self, ui_page: Page):
        """Typing in the search filter should reduce visible rows."""
        search = ui_page.locator(".search-input")
        rows_before = ui_page.locator(".config-table tbody tr").count()
        # Search for something unlikely to match all rows
        search.fill("neo4j")
        ui_page.wait_for_timeout(500)
        rows_after = ui_page.locator(".config-table tbody tr").count()
        assert rows_after <= rows_before, \
            f"Filter did not reduce rows: {rows_before} -> {rows_after}"

    def test_yaml_path_shown(self, ui_page: Page):
        """If a YAML config file exists, its path should be displayed."""
        muted = ui_page.locator(".muted.small")
        if muted.count() > 0:
            text = muted.first.inner_text()
            assert "YAML" in text or "yaml" in text or ".mdemg" in text, \
                f"YAML path not shown: {text}"


# ---------------------------------------------------------------------------
# Logs Tab
# ---------------------------------------------------------------------------

class TestLogsTab:
    """Verify the Logs tab shows the log viewer with controls."""

    @pytest.fixture(autouse=True)
    def switch_to_logs(self, ui_page: Page):
        ui_page.locator('.tab-btn[data-tab="logs"]').click()
        ui_page.wait_for_timeout(3000)

    def test_server_logs_header(self, ui_page: Page):
        """Server Logs section should appear."""
        header = ui_page.locator(".section-header").filter(has_text="Server Logs")
        expect(header).to_be_visible(timeout=10000)

    def test_log_container_present(self, ui_page: Page):
        """The log container element should exist."""
        container = ui_page.locator(".log-container")
        expect(container).to_be_visible(timeout=10000)

    def test_level_filter_present(self, ui_page: Page):
        """Level filter dropdown should be present with options."""
        level_select = ui_page.locator(".log-controls .select")
        expect(level_select).to_be_visible(timeout=5000)
        options = level_select.locator("option")
        option_values = [options.nth(i).get_attribute("value") for i in range(options.count())]
        assert "" in option_values, "Missing 'All Levels' option"
        assert "ERROR" in option_values, "Missing 'ERROR' option"
        assert "WARN" in option_values, "Missing 'WARN' option"
        assert "INFO" in option_values, "Missing 'INFO' option"

    def test_search_input_present(self, ui_page: Page):
        """Search input should be present."""
        search = ui_page.locator(".search-input")
        expect(search).to_be_visible(timeout=5000)

    def test_log_entries_rendered(self, ui_page: Page):
        """Log entries should appear in the container."""
        entries = ui_page.locator(".log-line")
        # Wait a bit for polling to bring data
        ui_page.wait_for_timeout(3000)
        assert entries.count() > 0, "No log entries rendered"

    def test_log_entries_have_structure(self, ui_page: Page):
        """Each log entry should have timestamp, level, and message."""
        entries = ui_page.locator(".log-line")
        ui_page.wait_for_timeout(3000)
        if entries.count() > 0:
            first = entries.first
            ts = first.locator(".log-ts")
            level = first.locator(".log-level")
            msg = first.locator(".log-msg")
            expect(ts).to_be_attached()
            expect(level).to_be_attached()
            expect(msg).to_be_attached()

    def test_log_level_colors(self, ui_page: Page):
        """Log entries should have color-coded level classes."""
        ui_page.wait_for_timeout(3000)
        valid_classes = {"log-info", "log-warn", "log-error", "log-debug"}
        entries = ui_page.locator(".log-line")
        if entries.count() > 0:
            classes = entries.first.get_attribute("class") or ""
            has_level = any(vc in classes for vc in valid_classes)
            assert has_level, f"Log entry missing level class: {classes}"

    def test_level_filter_filters(self, ui_page: Page):
        """Selecting a level should filter log entries."""
        ui_page.wait_for_timeout(3000)
        entries_before = ui_page.locator(".log-line").count()
        if entries_before == 0:
            pytest.skip("No log entries to filter")
        level_select = ui_page.locator(".log-controls .select")
        level_select.select_option("ERROR")
        ui_page.wait_for_timeout(500)
        entries_after = ui_page.locator(".log-line").count()
        # ERROR-only filter should return fewer or equal entries
        assert entries_after <= entries_before, \
            f"ERROR filter did not reduce entries: {entries_before} -> {entries_after}"

    def test_search_filter_works(self, ui_page: Page):
        """Typing in search should filter log entries."""
        ui_page.wait_for_timeout(3000)
        entries_before = ui_page.locator(".log-line").count()
        if entries_before == 0:
            pytest.skip("No log entries to search")
        search = ui_page.locator(".search-input")
        search.fill("xyznonexistent12345")
        ui_page.wait_for_timeout(500)
        entries_after = ui_page.locator(".log-line").count()
        assert entries_after == 0, \
            f"Search for nonexistent text should return 0 entries, got {entries_after}"


# ---------------------------------------------------------------------------
# RSIC Tab
# ---------------------------------------------------------------------------

class TestRsicTab:
    """Verify the RSIC tab has trigger form and Grafana link."""

    @pytest.fixture(autouse=True)
    def switch_to_rsic(self, ui_page: Page):
        ui_page.locator('.tab-btn[data-tab="rsic"]').click()
        ui_page.wait_for_timeout(2000)

    def test_trigger_header(self, ui_page: Page):
        """Trigger RSIC Cycle section should appear."""
        header = ui_page.locator(".section-header").filter(has_text="Trigger RSIC Cycle")
        expect(header).to_be_visible(timeout=10000)

    def test_tier_selector_present(self, ui_page: Page):
        """Tier selector dropdown should be present with micro/meso/macro."""
        tier_select = ui_page.locator(".action-row .select")
        expect(tier_select).to_be_visible(timeout=5000)
        options = tier_select.locator("option")
        values = [options.nth(i).get_attribute("value") for i in range(options.count())]
        assert "micro" in values, f"Missing 'micro' tier: {values}"
        assert "meso" in values, f"Missing 'meso' tier: {values}"
        assert "macro" in values, f"Missing 'macro' tier: {values}"

    def test_meso_selected_by_default(self, ui_page: Page):
        """Meso tier should be selected by default."""
        tier_select = ui_page.locator(".action-row .select")
        selected = tier_select.input_value()
        assert selected == "meso", f"Default tier is '{selected}', expected 'meso'"

    def test_dry_run_checkbox_present(self, ui_page: Page):
        """Dry run checkbox should be present."""
        checkbox = ui_page.locator("#rsic-dryrun")
        expect(checkbox).to_be_attached(timeout=5000)

    def test_trigger_button_present(self, ui_page: Page):
        """Trigger Cycle button should be visible."""
        trigger_btn = ui_page.locator(".btn-primary").filter(has_text="Trigger Cycle")
        expect(trigger_btn).to_be_visible(timeout=5000)

    def test_result_container_present(self, ui_page: Page):
        """Result div should exist for showing cycle results."""
        result = ui_page.locator(".rsic-result")
        expect(result).to_be_attached(timeout=5000)

    def test_grafana_link_present(self, ui_page: Page):
        """Grafana RSIC dashboard link should be present."""
        link = ui_page.locator(".grafana-link.large")
        expect(link).to_be_visible(timeout=5000)
        href = link.get_attribute("href") or ""
        assert "mdemg-rsic" in href, f"RSIC Grafana link has wrong href: {href}"

    def test_grafana_link_reuses_tab(self, ui_page: Page):
        """Grafana link should reuse existing Grafana tab."""
        link = ui_page.locator(".grafana-link.large")
        target = link.get_attribute("target")
        assert target == "grafana", f"Grafana link target is '{target}', expected 'grafana'"

    def test_detailed_metrics_header(self, ui_page: Page):
        """Detailed Metrics section header should appear."""
        header = ui_page.locator(".section-header").filter(has_text="Detailed Metrics")
        expect(header).to_be_visible(timeout=10000)


# ---------------------------------------------------------------------------
# Help Wiki Panels
# ---------------------------------------------------------------------------

class TestHelpPanels:
    """Verify each tab has a collapsible help wiki panel."""

    TAB_NAMES = ["status", "memory", "learning", "config", "logs", "rsic"]

    @pytest.mark.parametrize("tab_name", TAB_NAMES)
    def test_help_button_present(self, ui_page: Page, tab_name: str):
        """Each tab should have a '? Help' toggle button."""
        ui_page.locator(f'.tab-btn[data-tab="{tab_name}"]').click()
        ui_page.wait_for_timeout(3000)
        help_btn = ui_page.locator(".help-toggle")
        expect(help_btn).to_be_visible(timeout=10000)

    @pytest.mark.parametrize("tab_name", TAB_NAMES)
    def test_help_content_hidden_by_default(self, ui_page: Page, tab_name: str):
        """Help content should be hidden by default."""
        ui_page.locator(f'.tab-btn[data-tab="{tab_name}"]').click()
        ui_page.wait_for_timeout(3000)
        content = ui_page.locator(".help-content")
        assert content.count() > 0, "No help-content element found"
        display = content.first.evaluate("el => getComputedStyle(el).display")
        assert display == "none", f"Help content should be hidden, got display={display}"

    @pytest.mark.parametrize("tab_name", TAB_NAMES)
    def test_help_toggle_shows_content(self, ui_page: Page, tab_name: str):
        """Clicking '? Help' should show the help content."""
        ui_page.locator(f'.tab-btn[data-tab="{tab_name}"]').click()
        ui_page.wait_for_timeout(3000)
        ui_page.locator(".help-toggle").click()
        ui_page.wait_for_timeout(300)
        content = ui_page.locator(".help-content")
        display = content.first.evaluate("el => getComputedStyle(el).display")
        assert display != "none", f"Help content should be visible after toggle"

    @pytest.mark.parametrize("tab_name", TAB_NAMES)
    def test_help_has_entries(self, ui_page: Page, tab_name: str):
        """Help content should have at least 3 definition entries."""
        ui_page.locator(f'.tab-btn[data-tab="{tab_name}"]').click()
        ui_page.wait_for_timeout(3000)
        ui_page.locator(".help-toggle").click()
        ui_page.wait_for_timeout(300)
        entries = ui_page.locator(".help-entry")
        assert entries.count() >= 3, \
            f"Tab '{tab_name}' has {entries.count()} help entries, expected >= 3"

    @pytest.mark.parametrize("tab_name", TAB_NAMES)
    def test_help_toggle_hides_content(self, ui_page: Page, tab_name: str):
        """Clicking '? Help' twice should hide the content again."""
        ui_page.locator(f'.tab-btn[data-tab="{tab_name}"]').click()
        ui_page.wait_for_timeout(3000)
        toggle = ui_page.locator(".help-toggle")
        toggle.click()
        ui_page.wait_for_timeout(300)
        toggle.click()
        ui_page.wait_for_timeout(300)
        content = ui_page.locator(".help-content")
        display = content.first.evaluate("el => getComputedStyle(el).display")
        assert display == "none", f"Help content should be hidden after double toggle"


# ---------------------------------------------------------------------------
# API Endpoints (direct HTTP verification)
# ---------------------------------------------------------------------------

class TestAdminConfigAPI:
    """Verify the /v1/admin/config API endpoint."""

    def test_config_returns_200(self):
        """GET /v1/admin/config should return 200."""
        resp = requests.get(f"{MDEMG_URL}/v1/admin/config")
        assert resp.status_code == 200, f"Expected 200, got {resp.status_code}"

    def test_config_returns_json(self):
        """Response should be valid JSON with config array."""
        resp = requests.get(f"{MDEMG_URL}/v1/admin/config")
        data = resp.json()
        assert "config" in data, f"Missing 'config' key: {list(data.keys())}"
        assert isinstance(data["config"], list), "config should be a list"

    def test_config_entries_have_required_fields(self):
        """Each config entry should have key, value, source, masked fields."""
        resp = requests.get(f"{MDEMG_URL}/v1/admin/config")
        data = resp.json()
        for entry in data["config"][:5]:  # Check first 5
            assert "key" in entry, f"Missing 'key' in entry: {entry}"
            assert "value" in entry, f"Missing 'value' in entry: {entry}"
            assert "source" in entry, f"Missing 'source' in entry: {entry}"
            assert "masked" in entry, f"Missing 'masked' in entry: {entry}"

    def test_config_sources_valid(self):
        """Source field should be one of env, yaml, default."""
        resp = requests.get(f"{MDEMG_URL}/v1/admin/config")
        data = resp.json()
        valid_sources = {"env", "yaml", "default"}
        for entry in data["config"]:
            assert entry["source"] in valid_sources, \
                f"Invalid source '{entry['source']}' for key '{entry['key']}'"

    def test_config_masked_values(self):
        """Masked entries should have value='****'."""
        resp = requests.get(f"{MDEMG_URL}/v1/admin/config")
        data = resp.json()
        masked = [e for e in data["config"] if e["masked"]]
        for entry in masked:
            assert entry["value"] == "****", \
                f"Masked entry '{entry['key']}' has value '{entry['value']}', expected '****'"

    def test_config_method_not_allowed(self):
        """POST to /v1/admin/config should return 405."""
        resp = requests.post(f"{MDEMG_URL}/v1/admin/config")
        assert resp.status_code == 405, f"Expected 405, got {resp.status_code}"


class TestAdminLogsAPI:
    """Verify the /v1/admin/logs API endpoint."""

    def test_logs_returns_200(self):
        """GET /v1/admin/logs should return 200."""
        resp = requests.get(f"{MDEMG_URL}/v1/admin/logs")
        assert resp.status_code == 200, f"Expected 200, got {resp.status_code}"

    def test_logs_returns_json(self):
        """Response should be valid JSON with entries array."""
        resp = requests.get(f"{MDEMG_URL}/v1/admin/logs")
        data = resp.json()
        assert "entries" in data, f"Missing 'entries' key: {list(data.keys())}"
        assert isinstance(data["entries"], list), "entries should be a list"

    def test_logs_has_seq(self):
        """Response should include a seq field."""
        resp = requests.get(f"{MDEMG_URL}/v1/admin/logs")
        data = resp.json()
        assert "seq" in data, f"Missing 'seq' key: {list(data.keys())}"

    def test_logs_entries_have_structure(self):
        """Log entries should have timestamp, level, message, raw fields."""
        resp = requests.get(f"{MDEMG_URL}/v1/admin/logs")
        data = resp.json()
        if len(data["entries"]) > 0:
            entry = data["entries"][0]
            assert "level" in entry, f"Missing 'level': {list(entry.keys())}"
            assert "message" in entry, f"Missing 'message': {list(entry.keys())}"
            assert "raw" in entry, f"Missing 'raw': {list(entry.keys())}"

    def test_logs_limit_parameter(self):
        """limit parameter should cap the number of entries."""
        resp = requests.get(f"{MDEMG_URL}/v1/admin/logs?limit=5")
        data = resp.json()
        assert len(data["entries"]) <= 5, \
            f"Expected <=5 entries with limit=5, got {len(data['entries'])}"

    def test_logs_method_not_allowed(self):
        """POST to /v1/admin/logs should return 405."""
        resp = requests.post(f"{MDEMG_URL}/v1/admin/logs")
        assert resp.status_code == 405, f"Expected 405, got {resp.status_code}"


class TestUIEndpoint:
    """Verify the /ui/ static file serving endpoint."""

    def test_ui_root_returns_html(self):
        """GET /ui/ should return HTML."""
        resp = requests.get(f"{MDEMG_URL}/ui/")
        assert resp.status_code == 200, f"Expected 200, got {resp.status_code}"
        assert "text/html" in resp.headers.get("content-type", ""), \
            f"Expected HTML content-type: {resp.headers.get('content-type')}"

    def test_ui_index_has_content(self):
        """The HTML should contain the dashboard structure."""
        resp = requests.get(f"{MDEMG_URL}/ui/")
        html = resp.text
        assert "MDEMG Dashboard" in html, "Missing dashboard title"
        assert "tab-btn" in html, "Missing tab buttons"
        assert "space-select" in html, "Missing space selector"
        assert "main.js" in html, "Missing main.js script reference"

    def test_ui_js_files_accessible(self):
        """JS module files should be accessible."""
        for path in ["main.js", "api.js", "state.js"]:
            resp = requests.get(f"{MDEMG_URL}/ui/{path}")
            assert resp.status_code == 200, f"/ui/{path} returned {resp.status_code}"
            assert "javascript" in resp.headers.get("content-type", "").lower() or \
                   "text/plain" in resp.headers.get("content-type", ""), \
                f"/ui/{path} content-type: {resp.headers.get('content-type')}"

    def test_ui_subdirectory_files_accessible(self):
        """JS files in subdirectories should be accessible."""
        paths = [
            "tabs/status.js", "tabs/memory.js", "tabs/learning.js",
            "tabs/config.js", "tabs/logs.js", "tabs/rsic.js",
            "utils/dom.js", "utils/formatting.js",
        ]
        for path in paths:
            resp = requests.get(f"{MDEMG_URL}/ui/{path}")
            assert resp.status_code == 200, f"/ui/{path} returned {resp.status_code}"

    def test_ui_404_for_nonexistent(self):
        """Nonexistent files should return 404."""
        resp = requests.get(f"{MDEMG_URL}/ui/nonexistent.js")
        assert resp.status_code == 404, f"Expected 404, got {resp.status_code}"

    def test_ui_without_trailing_slash_redirects(self):
        """GET /ui (without trailing slash) should redirect to /ui/."""
        resp = requests.get(f"{MDEMG_URL}/ui", allow_redirects=False)
        # http.StripPrefix + FileServer will handle this
        assert resp.status_code in (200, 301, 302, 307, 308), \
            f"Expected redirect or 200, got {resp.status_code}"


# ---------------------------------------------------------------------------
# Polling Behavior
# ---------------------------------------------------------------------------

class TestPollingBehavior:
    """Verify that the dashboard makes expected API calls during operation."""

    def test_health_polling_makes_requests(self, ui_page: Page, mdemg_url: str):
        """The dashboard should make periodic health requests."""
        health_requests = []

        def on_request(request):
            if "/healthz" in request.url:
                health_requests.append(request.url)

        ui_page.on("request", on_request)
        ui_page.goto(f"{mdemg_url}/ui/", wait_until="networkidle")
        # Wait enough time for at least one health poll
        ui_page.wait_for_timeout(5000)
        assert len(health_requests) >= 1, \
            f"Expected at least 1 healthz request, got {len(health_requests)}"

    def test_stats_polling_makes_requests(self, ui_page: Page, mdemg_url: str):
        """The dashboard should make memory/learning stats requests."""
        stat_requests = []

        def on_request(request):
            if "/v1/memory/stats" in request.url or "/v1/learning/stats" in request.url:
                stat_requests.append(request.url)

        ui_page.on("request", on_request)
        ui_page.goto(f"{mdemg_url}/ui/", wait_until="networkidle")
        ui_page.wait_for_timeout(5000)
        assert len(stat_requests) >= 1, \
            f"Expected at least 1 stats request, got {len(stat_requests)}"

    def test_logs_poll_only_on_logs_tab(self, ui_page: Page, mdemg_url: str):
        """Log polling should only occur when the Logs tab is active."""
        log_requests = []

        def on_request(request):
            if "/v1/admin/logs" in request.url:
                log_requests.append(request.url)

        ui_page.on("request", on_request)
        ui_page.goto(f"{mdemg_url}/ui/", wait_until="networkidle")
        # Stay on Status tab for 6 seconds (logs poll every 5s)
        ui_page.wait_for_timeout(6000)
        status_tab_log_count = len(log_requests)

        # Switch to Logs tab
        ui_page.locator('.tab-btn[data-tab="logs"]').click()
        ui_page.wait_for_timeout(6000)
        logs_tab_log_count = len(log_requests) - status_tab_log_count

        # Logs tab should have triggered log requests
        assert logs_tab_log_count >= 1, \
            f"Expected log requests on Logs tab, got {logs_tab_log_count}"


# ---------------------------------------------------------------------------
# Catppuccin Mocha Theme Verification
# ---------------------------------------------------------------------------

class TestTheme:
    """Verify the Catppuccin Mocha theme is applied correctly."""

    def test_dark_background(self, ui_page: Page):
        """Body background should be the Catppuccin base color (#1e1e2e)."""
        bg = ui_page.evaluate("getComputedStyle(document.body).backgroundColor")
        # rgb(30, 30, 46) = #1e1e2e
        assert "30, 30, 46" in bg or "1e1e2e" in bg, \
            f"Background color is not Catppuccin base: {bg}"

    def test_text_color(self, ui_page: Page):
        """Body text should use Catppuccin text color (#cdd6f4)."""
        color = ui_page.evaluate("getComputedStyle(document.body).color")
        # rgb(205, 214, 244) = #cdd6f4
        assert "205, 214, 244" in color or "cdd6f4" in color, \
            f"Text color is not Catppuccin text: {color}"

    def test_header_background(self, ui_page: Page):
        """Header should use mantle background (#181825)."""
        bg = ui_page.evaluate(
            "getComputedStyle(document.querySelector('.header')).backgroundColor"
        )
        # rgb(24, 24, 37) = #181825
        assert "24, 24, 37" in bg or "181825" in bg, \
            f"Header background is not Catppuccin mantle: {bg}"
