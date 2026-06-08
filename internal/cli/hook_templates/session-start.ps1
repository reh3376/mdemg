# MDEMG session-start hook (installed by mdemg hooks install)
# Hook: SessionStart — restore CMS memory context on every session start

$ErrorActionPreference = 'SilentlyContinue'

$MDEMG_URL = if ($env:MDEMG_URL) { $env:MDEMG_URL } else { "{{MDEMG_URL}}" }
$SPACE_ID = "{{SPACE_ID}}"
# Resolve SessionID per conversation: MDEMG_SESSION_ID env > Claude Code stdin
# session_id > ~/.mdemg/.claude-session > claude-core. Publish it for the agent.
$rawInput = [Console]::In.ReadToEnd()
try { $hookInput = $rawInput | ConvertFrom-Json } catch { $hookInput = $null }
$SESSION_ID = if ($env:MDEMG_SESSION_ID) { $env:MDEMG_SESSION_ID } else { "" }
if (-not $SESSION_ID -and $hookInput) { $SESSION_ID = $hookInput.session_id }
if (-not $SESSION_ID) { try { $SESSION_ID = (Get-Content "$env:USERPROFILE\.mdemg\.claude-session" -Raw | ConvertFrom-Json).session_id } catch {} }
if (-not $SESSION_ID) { $SESSION_ID = "claude-core" }
try {
    $sessDir = "$env:USERPROFILE\.mdemg"
    if (-not (Test-Path $sessDir)) { New-Item -ItemType Directory -Path $sessDir -Force | Out-Null }
    @{session_id=$SESSION_ID; ts=[int][double]::Parse((Get-Date -UFormat %s))} | ConvertTo-Json -Compress | Set-Content "$sessDir\.claude-session"
} catch {}
$MAX_OBS = 10

# Check if MDEMG server is reachable
try {
    $null = Invoke-RestMethod -Uri "$MDEMG_URL/healthz" -TimeoutSec 2
} catch {
    Write-Output @"
Warning: CMS DISCONNECTED — MDEMG server is not running.
Memory is unavailable. You are operating without persistent context.
Warn the user: "CMS unavailable — memory disconnected."
Do NOT make irreversible decisions without user confirmation.
"@
    exit 0
}

# Call resume endpoint
try {
    $body = @{
        space_id = $SPACE_ID
        session_id = $SESSION_ID
        max_observations = $MAX_OBS
    } | ConvertTo-Json
    $resume = Invoke-RestMethod -Uri "$MDEMG_URL/v1/conversation/resume" -Method Post -ContentType "application/json" -Body $body -TimeoutSec 10
} catch {
    Write-Output "Warning: CMS resume failed — memory may be incomplete."
    exit 0
}

# Extract key fields
$obsCount = if ($resume.observations) { $resume.observations.Count } else { 0 }
$themeCount = if ($resume.themes) { $resume.themes.Count } else { 0 }
$conceptCount = if ($resume.emergent_concepts) { $resume.emergent_concepts.Count } else { 0 }
$summary = if ($resume.summary) { $resume.summary } else { "No summary available" }

# Anomaly detection: resume returned empty but space has data
if ($obsCount -eq 0) {
    try {
        $stats = Invoke-RestMethod -Uri "$MDEMG_URL/v1/memory/stats?space_id=$SPACE_ID" -TimeoutSec 3
        $nodeCount = if ($stats.memory_count) { $stats.memory_count } else { 0 }

        if ($nodeCount -gt 0) {
            Write-Output @"

!! CRITICAL: MEMORY RETURNED EMPTY !!
Resume returned 0 observations for active space.
Space has data but nothing was retrieved.

MANDATORY INVESTIGATION:
  1. POST /v1/self-improve/assess {"space_id":"$SPACE_ID","tier":"micro"}
  2. GET /v1/memory/stats?space_id=$SPACE_ID

DO NOT PROCEED until investigated.
"@

            # Background: trigger self-assess and log anomaly
            Start-Job -ScriptBlock {
                param($url, $sid, $sessId, $nc)
                try { Invoke-RestMethod -Uri "$url/v1/self-improve/assess" -Method Post -ContentType "application/json" -Body (@{space_id=$sid; tier="micro"} | ConvertTo-Json) -TimeoutSec 8 } catch {}
                try { Invoke-RestMethod -Uri "$url/v1/conversation/observe" -Method Post -ContentType "application/json" -Body (@{space_id=$sid; session_id=$sessId; content="ANOMALY: Resume returned 0 observations but space has $nc nodes. Embedder or query failure suspected."; obs_type="error"; tags=@("anomaly","empty-resume")} | ConvertTo-Json) -TimeoutSec 5 } catch {}
            } -ArgumentList $MDEMG_URL, $SPACE_ID, $SESSION_ID, $nodeCount | Out-Null
        }
    } catch {}
}

# Build context injection
Write-Output "=== CMS MEMORY RESTORED ==="
Write-Output "Observations: $obsCount | Themes: $themeCount | Concepts: $conceptCount"
Write-Output "Summary: $summary"

if ($obsCount -gt 0) {
    Write-Output ""
    Write-Output "Recent observations:"
    foreach ($obs in $resume.observations) {
        $type = if ($obs.obs_type) { $obs.obs_type } else { "unknown" }
        $content = if ($obs.content) { $obs.content } elseif ($obs.summary) { $obs.summary } else { "no content" }
        Write-Output "  * [$type] $content"
    }
}

if ($themeCount -gt 0) {
    Write-Output ""
    Write-Output "Active themes:"
    foreach ($theme in $resume.themes) {
        $name = if ($theme.name) { $theme.name } else { "unnamed" }
        $members = if ($theme.member_count) { $theme.member_count } else { 0 }
        Write-Output "  * $name (members: $members)"
    }
}

Write-Output ""
Write-Output "=== END CMS CONTEXT ==="

# RSIC health assessment
try {
    $rsicBody = @{ space_id = $SPACE_ID; tier = "micro" } | ConvertTo-Json
    $rsic = Invoke-RestMethod -Uri "$MDEMG_URL/v1/self-improve/assess" -Method Post -ContentType "application/json" -Body $rsicBody -TimeoutSec 8

    $overall = if ($rsic.overall_health) { $rsic.overall_health } else { "?" }
    $retrieval = if ($rsic.retrieval_quality) { $rsic.retrieval_quality } else { "?" }
    $memory = if ($rsic.memory_health) { $rsic.memory_health } else { "?" }
    $edge = if ($rsic.edge_health) { $rsic.edge_health } else { "?" }
    $learnPhase = if ($rsic.learning_phase) { $rsic.learning_phase } else { "?" }
    $orphanRatio = if ($rsic.orphan_ratio) { $rsic.orphan_ratio } else { "?" }

    Write-Output ""
    Write-Output "=== RSIC HEALTH ==="
    Write-Output "Overall: $overall | Retrieval: $retrieval | Memory: $memory | Edge: $edge"
    Write-Output "Learning: $learnPhase | Orphan ratio: $orphanRatio"
    Write-Output "=== END RSIC HEALTH ==="
} catch {}

# Reinforce recalled observations + graduation check in background
if ($obsCount -gt 0) {
    Start-Job -ScriptBlock {
        param($url, $sid, $sessId, $oc, $tc, $cc)
        try { Invoke-RestMethod -Uri "$url/v1/conversation/observe" -Method Post -ContentType "application/json" -Body (@{space_id=$sid; session_id=$sessId; content="Session resumed. $oc observations, $tc themes, $cc concepts recalled. Memory continuity maintained."; obs_type="context"; tags=@("session-resume","auto-reinforce")} | ConvertTo-Json) -TimeoutSec 5 } catch {}
        try { Invoke-RestMethod -Uri "$url/v1/conversation/graduate" -Method Post -ContentType "application/json" -Body (@{space_id=$sid} | ConvertTo-Json) -TimeoutSec 5 } catch {}
    } -ArgumentList $MDEMG_URL, $SPACE_ID, $SESSION_ID, $obsCount, $themeCount, $conceptCount | Out-Null
}
