# MDEMG prompt-context hook (installed by mdemg hooks install)
# Hook: UserPromptSubmit — recall CMS context + Jiminy guidance for each prompt

$ErrorActionPreference = 'SilentlyContinue'

$MDEMG_URL = if ($env:MDEMG_URL) { $env:MDEMG_URL } else { "{{MDEMG_URL}}" }
$SPACE_ID = "{{SPACE_ID}}"

# Read hook input from stdin
$rawInput = [Console]::In.ReadToEnd()
if (-not $rawInput) { exit 0 }

try {
    $hookInput = $rawInput | ConvertFrom-Json
    $userPrompt = $hookInput.user_prompt
} catch {
    exit 0
}

if (-not $userPrompt) { exit 0 }

# Resolve SessionID per conversation: MDEMG_SESSION_ID env > Claude Code stdin
# session_id > ~/.mdemg/.claude-session > claude-core. Publish it for the agent.
$SESSION_ID = if ($env:MDEMG_SESSION_ID) { $env:MDEMG_SESSION_ID } else { "" }
if (-not $SESSION_ID) { $SESSION_ID = $hookInput.session_id }
if (-not $SESSION_ID) { try { $SESSION_ID = (Get-Content "$env:USERPROFILE\.mdemg\.claude-session" -Raw | ConvertFrom-Json).session_id } catch {} }
if (-not $SESSION_ID) { $SESSION_ID = "claude-core" }
try {
    $sessDir = "$env:USERPROFILE\.mdemg"
    if (-not (Test-Path $sessDir)) { New-Item -ItemType Directory -Path $sessDir -Force | Out-Null }
    @{session_id=$SESSION_ID; ts=[int][double]::Parse((Get-Date -UFormat %s))} | ConvertTo-Json -Compress | Set-Content "$sessDir\.claude-session"
} catch {}

# Skip very short prompts (commands like "y", "ok", etc.)
if ($userPrompt.Length -lt 15) { exit 0 }

# Check server is up (fast fail)
try {
    $null = Invoke-RestMethod -Uri "$MDEMG_URL/healthz" -TimeoutSec 1
} catch {
    exit 0
}

# Recall relevant context from CMS
try {
    $recallBody = @{
        space_id = $SPACE_ID
        query = $userPrompt
        top_k = 5
        include_themes = $true
        include_concepts = $true
    } | ConvertTo-Json
    $recall = Invoke-RestMethod -Uri "$MDEMG_URL/v1/conversation/recall" -Method Post -ContentType "application/json" -Body $recallBody -TimeoutSec 8
} catch {
    exit 0
}

# Check if there are results
$results = if ($recall -is [System.Array]) { $recall } elseif ($recall.results) { $recall.results } else { @() }
$resultCount = $results.Count

if ($resultCount -eq 0) {
    if ($userPrompt.Length -gt 15) {
        Write-Output "!! CMS RECALL EMPTY — No relevant memory found for this query."
        Write-Output "!! Consider: POST /v1/conversation/observe to record this topic."
    }
    try {
        $health = Invoke-RestMethod -Uri "$MDEMG_URL/v1/conversation/session/health?session_id=$SESSION_ID" -TimeoutSec 1
        $hScore = if ($health.health_score) { $health.health_score } else { "?" }
        $hObs = if ($health.observations_since_resume) { $health.observations_since_resume } else { "?" }
        Write-Output "[Session health: $hScore | obs: $hObs]"
    } catch {}
    exit 0
}

# Format relevant context
Write-Output "=== CMS RECALL (relevant to this prompt) ==="

foreach ($r in $results) {
    $type = if ($r.type) { $r.type } elseif ($r.obs_type) { $r.obs_type } else { "memory" }
    $score = if ($r.score) { [string]$r.score; if ($score.Length -gt 4) { $score = $score.Substring(0, 4) } } else { "?" }
    $content = if ($r.content) { $r.content } elseif ($r.summary) { $r.summary } else { "no content" }
    if ($content.Length -gt 200) { $content = $content.Substring(0, 200) }
    Write-Output "  * [$type] (score: $score) $content"
}

Write-Output "=== END CMS RECALL ==="

# --- Jiminy inner voice guidance ---
# Always attempts the call. Server returns 503 if disabled — Invoke-RestMethod fails silently.
try {
    $guidanceBody = @{
        space_id = $SPACE_ID
        context = $userPrompt
        session_id = $SESSION_ID
    } | ConvertTo-Json
    $guidance = Invoke-RestMethod -Uri "$MDEMG_URL/v1/jiminy/guide" -Method Post -ContentType "application/json" -Body $guidanceBody -TimeoutSec 6

    if ($guidance.data.prompt_augmentation) {
        Write-Output ""
        Write-Output $guidance.data.prompt_augmentation
    }
} catch {}

# --- Reinforce recalled observations via retrieval co-activation ---
Start-Job -ScriptBlock {
    param($url, $sid, $prompt)
    try {
        Invoke-RestMethod -Uri "$url/v1/memory/retrieve" -Method Post -ContentType "application/json" -Body (@{space_id=$sid; query_text=$prompt; candidate_k=10; top_k=5; hop_depth=2} | ConvertTo-Json) -TimeoutSec 5
    } catch {}
} -ArgumentList $MDEMG_URL, $SPACE_ID, $userPrompt | Out-Null

# Session health ribbon
try {
    $health = Invoke-RestMethod -Uri "$MDEMG_URL/v1/conversation/session/health?session_id=$SESSION_ID" -TimeoutSec 1
    $hScore = if ($health.health_score) { $health.health_score } else { "?" }
    $hObs = if ($health.observations_since_resume) { $health.observations_since_resume } else { "?" }
    Write-Output "[Session health: $hScore | obs: $hObs]"
} catch {}
