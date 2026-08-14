"""JIMINY-RULES-UI-001 Epic 5: Rules tab browser e2e.

Non-destructive against the mdemg-dev space (READ-only exercise + the
Add-form UX; the WRITE-flow live end-to-end is verified via curl smoke
on a scratch space in Epic 5's shell smoke — see sprint post). This
suite verifies the UI mechanics: tab loads, filters work, detail opens,
Add form toggles + validates, dedup-warn shape renders on 409.

Runs against a running mdemg server at MDEMG_URL (default
http://localhost:9999) with playwright-python installed.
"""

import pytest
from playwright.sync_api import Page


class TestRulesTab:
    """Rules tab: list + filter + detail + Add form + dedup UX."""

    def _open_rules(self, ui_page: Page):
        ui_page.locator('.tab-btn[data-tab="rules"]').click()
        # Give the async load() one poll cycle.
        ui_page.wait_for_timeout(2000)

    def test_rules_tab_loads_header(self, ui_page: Page):
        """Tab click → renders the 'Jiminy Rules' section header."""
        self._open_rules(ui_page)
        # The tab's toolbar section header includes 'Jiminy Rules — space='
        assert ui_page.locator("text=Jiminy Rules —").count() > 0, "Rules tab header did not render"

    def test_rules_tab_renders_list_or_empty(self, ui_page: Page):
        """List view shows either a table with rows OR the empty-state text."""
        self._open_rules(ui_page)
        table = ui_page.locator("table.config-table")
        empty = ui_page.locator("text=No rules match")
        assert table.count() > 0 or empty.count() > 0, "Rules tab rendered neither a table nor an empty state"

    def test_rules_add_button_toggles_form(self, ui_page: Page):
        """+ Add Rule button toggles the Add form (Publish button appears/disappears)."""
        self._open_rules(ui_page)
        add_btn = ui_page.get_by_role("button", name="+ Add Rule")
        if add_btn.count() == 0:
            pytest.skip("Add Rule button not found — Rules tab may not have rendered")
        add_btn.click()
        ui_page.wait_for_timeout(500)
        publish_btn = ui_page.get_by_role("button", name="Publish")
        assert publish_btn.count() > 0, "Publish button did not appear after clicking + Add Rule"
        # Cancel via the newly-labeled 'Cancel Add' button in the toolbar
        # (label swaps between '+ Add Rule' and 'Cancel Add' — locator matches text).
        ui_page.get_by_role("button", name="Cancel Add").first.click()
        ui_page.wait_for_timeout(500)
        assert ui_page.get_by_role("button", name="Publish").count() == 0, "Publish button still visible after Cancel Add"

    def test_rules_add_form_validates_empty_content(self, ui_page: Page):
        """Publish with empty content shows the 'Content is required' error."""
        self._open_rules(ui_page)
        add_btn = ui_page.get_by_role("button", name="+ Add Rule")
        if add_btn.count() == 0:
            pytest.skip("Add Rule button not found")
        add_btn.click()
        ui_page.wait_for_timeout(500)
        publish_btn = ui_page.get_by_role("button", name="Publish")
        if publish_btn.count() == 0:
            pytest.skip("Publish button did not render")
        publish_btn.click()
        ui_page.wait_for_timeout(500)
        assert ui_page.locator("text=Content is required").count() > 0, \
            "Empty-content submit did not surface validation error"

    def test_rules_detail_opens_on_row_click(self, ui_page: Page):
        """Row-click expands inline (accordion) — Close + Edit + Tombstone buttons appear."""
        self._open_rules(ui_page)
        rows = ui_page.locator("table.config-table tbody tr")
        if rows.count() == 0:
            pytest.skip("No rules in the corpus to click")
        rows.first.click()
        ui_page.wait_for_timeout(2000)
        # Accordion expansion — inline detail has Close + Edit + Tombstone buttons.
        assert ui_page.get_by_role("button", name="Close").count() > 0, "Close button missing on expanded row"
        # Edit button may be replaced by "(archived — un-tombstone via direct Cypher to edit)"
        # if the first row happens to be archived — either state is valid.
        has_edit = ui_page.get_by_role("button", name="Edit").count() > 0
        has_archived_note = ui_page.locator("text=un-tombstone via direct Cypher").count() > 0
        assert has_edit or has_archived_note, "Neither Edit button nor archived-note present on expanded row"

    def test_rules_search_narrows_list(self, ui_page: Page):
        """Search box filters the rendered list by node_id / code / content."""
        self._open_rules(ui_page)
        rows_before = ui_page.locator("table.config-table tbody tr").count()
        if rows_before < 2:
            pytest.skip("Need ≥2 rows to demonstrate search narrowing")
        # Type a token unlikely to match everything — use a code we know
        # exists in the corpus (never-direct-main-commits is stable per
        # JIMINY-CORPUS-003).
        search = ui_page.locator('input[placeholder*="Search"]')
        assert search.count() > 0, "Search input not rendered in toolbar"
        search.fill("never-direct-main")
        ui_page.wait_for_timeout(500)  # debounce is 200ms
        rows_after = ui_page.locator("table.config-table tbody tr").count()
        assert rows_after < rows_before, f"Search should narrow list: before={rows_before}, after={rows_after}"

    def test_rules_edit_button_toggles_form(self, ui_page: Page):
        """Edit button on an expanded LIVE rule → Save + Cancel appear; Cancel returns to view."""
        self._open_rules(ui_page)
        rows = ui_page.locator("table.config-table tbody tr")
        if rows.count() == 0:
            pytest.skip("No rules in the corpus")
        # Find a non-archived row; open it. In the mdemg-dev corpus the
        # first row is typically live post-JIMINY-CORPUS-003.
        rows.first.click()
        ui_page.wait_for_timeout(1500)
        edit_btn = ui_page.get_by_role("button", name="Edit")
        if edit_btn.count() == 0:
            pytest.skip("First row is archived; Edit button not applicable")
        # .first — accordion re-render can transiently keep the pre-render
        # DOM around for a tick; strict-mode single-match would flake.
        edit_btn.first.click()
        ui_page.wait_for_timeout(500)
        assert ui_page.get_by_role("button", name="Save").count() > 0, "Save button missing after Edit click"
        assert ui_page.locator("text=Editing").count() > 0, "Edit-mode header missing"
        # Cancel returns to view mode
        cancel_btns = ui_page.get_by_role("button", name="Cancel")
        cancel_btns.first.click()
        ui_page.wait_for_timeout(500)
        assert ui_page.get_by_role("button", name="Save").count() == 0, "Save button still visible after Cancel"

    def test_rules_help_panel_present(self, ui_page: Page):
        """Help panel documents the 5 concepts: Publish, Dedup, Tombstone, Immutable, Arc-safety."""
        self._open_rules(ui_page)
        # helpPanel renders a section with a title + entries; assert a couple
        # of the fixed strings are present.
        assert ui_page.locator("text=How the Rules tab works").count() > 0, "Help panel missing"
        # Key concepts we want operators to see (updated for UX-review revision):
        for concept in ["Search box", "Click a row", "Publish = live", "Edit = supersede", "Dedup warn", "Tombstone = soft delete", "Arc-safety flag"]:
            assert ui_page.locator(f"text={concept}").count() > 0, f"Help panel missing concept: {concept}"

    def test_rules_write_flag_off_disabled_state(self, ui_page: Page):
        """The toolbar names the flag + arc window in its help text."""
        self._open_rules(ui_page)
        assert ui_page.locator("text=JIMINY_RULES_UI_WRITE_ENABLED").count() > 0, \
            "Toolbar text did not mention the arc-safety flag"
        assert ui_page.locator("text=2026-08-19").count() > 0, \
            "Toolbar text did not mention the arc-window boundary date"
