# View the latest 5 check batches per check_name.
# Column headers come from t_check_meta field names, values printed raw, one color per column.
# Usage: powershell -ExecutionPolicy Bypass -File .\view_check_record.ps1

$MYSQL = @("exec", "-i", "mysql-slave1", "mysql", "-uroot", "-proot123456",
    "--default-character-set=utf8mb4", "-N", "-B", "-e")

$COLORS = @("Cyan", "Green", "Yellow", "Magenta", "Blue", "Red", "White", "DarkCyan", "DarkGreen", "DarkYellow", "Gray")

# Display width: ASCII=1, others(CJK etc)=2
function Get-Width([string]$s) {
    $w = 0
    foreach ($ch in $s.ToCharArray()) {
        if ([int]$ch -gt 127) { $w += 2 } else { $w += 1 }
    }
    return $w
}

function Pad-Display([string]$s, [int]$width) {
    return $s + (" " * [Math]::Max(0, $width - (Get-Width $s)))
}

# 1) Load meta definitions: check_id, check_name, field1_name..field10_name
$metaSql = "select check_id, check_name, field1_name, field2_name, field3_name, field4_name, field5_name, field6_name, field7_name, field8_name, field9_name, field10_name from order_db.t_check_meta order by check_id"
$metaLines = docker @MYSQL $metaSql 2>$null
if ($LASTEXITCODE -ne 0) { Write-Host "Query meta failed"; exit 1 }

foreach ($metaLine in $metaLines) {
    if ([string]::IsNullOrWhiteSpace($metaLine)) { continue }
    $m = $metaLine -split "`t", -1
    $checkId = $m[0]
    $checkName = $m[1]
    $names = @()
    for ($i = 2; $i -le 11; $i++) {
        $names += $(if ($i -lt $m.Count) { $m[$i] } else { "" })
    }

    # 2) Latest 5 records of this check task
    $recSql = "select field1_data, field2_data, field3_data, field4_data, field5_data, field6_data, field7_data, field8_data, field9_data, field10_data, create_time from order_db.t_check_record where check_id = $checkId order by record_id desc limit 5"
    $recLines = docker @MYSQL $recSql 2>$null
    if ($LASTEXITCODE -ne 0) { Write-Host "Query record failed for $checkName"; continue }

    # Build table: header = meta field names (skip empty ones), last column = create_time
    $colNames = @()
    foreach ($n in $names) { if (-not [string]::IsNullOrWhiteSpace($n)) { $colNames += $n } }
    $colNames += "create_time"

    $rows = @()
    foreach ($recLine in $recLines) {
        if ([string]::IsNullOrWhiteSpace($recLine)) { continue }
        $cells = $recLine -split "`t", -1
        $row = @()
        for ($i = 0; $i -lt $names.Count; $i++) {
            if (-not [string]::IsNullOrWhiteSpace($names[$i])) {
                $row += $(if ($i -lt $cells.Count) { $cells[$i] } else { "" })
            }
        }
        $row += $(if ($cells.Count -ge 11) { $cells[10] } else { "" })
        $rows += , $row
    }

    # 3) Compute column widths and print with one color per column
    $widths = @()
    for ($c = 0; $c -lt $colNames.Count; $c++) {
        $w = Get-Width $colNames[$c]
        foreach ($row in $rows) { $w = [Math]::Max($w, (Get-Width $row[$c])) }
        $widths += ($w + 2)
    }

    Write-Host ""
    Write-Host "== $checkId / $checkName ==" -ForegroundColor DarkGray
    for ($c = 0; $c -lt $colNames.Count; $c++) {
        Write-Host -NoNewline (Pad-Display $colNames[$c] $widths[$c]) -ForegroundColor $COLORS[$c % $COLORS.Count]
    }
    Write-Host ""
    foreach ($row in $rows) {
        for ($c = 0; $c -lt $colNames.Count; $c++) {
            Write-Host -NoNewline (Pad-Display $row[$c] $widths[$c]) -ForegroundColor $COLORS[$c % $COLORS.Count]
        }
        Write-Host ""
    }
    if ($rows.Count -eq 0) { Write-Host "(no records)" -ForegroundColor DarkGray }
}
