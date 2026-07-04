param(
    [string]$Branch = "main"
)

$ErrorActionPreference = "Stop"

Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    git push origin $Branch
    $command = "bash ~/bimmlite/scripts/deploy.sh"
    gcloud compute ssh free-server-forever --zone=us-central1-f --project=project-8fc391c2-e159-4215-800 --command $command
}
finally {
    Pop-Location
}
