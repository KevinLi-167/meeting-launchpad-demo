# 会议室入会服务 · 启动/停止开关（在跑就停，没跑就静默拉起）
#
# 用法：把本脚本与目标 exe 放在同一目录，右键「使用 PowerShell 运行」即可。
# 注 1：ps1 脚本必须保存为 `UTF-8 BOM`，否则中文会乱码。
# 注 2：本脚本用 $PSScriptRoot（脚本自身所在目录）定位 exe，不用 $PWD。
#       原因：任务计划程序里的「起始于」很容易配错，用 $PWD 会指向别的目录，
#       这与程序内部「数据文件固定放在 exe 同级目录」的取舍是同一条原则。

# ---- 1) 指定要操作的 exe ----
# 留空 = 在本脚本所在目录里自动查找（要求该目录下只有一个 exe）。
# 若需要指定具体实例，改成实际文件名，例如：
#   $exeFileName = "MeetingLaunchpad_v2.1-room1.exe"
$exeFileName = ""

$scriptDir = $PSScriptRoot
if (-not $scriptDir) {
    $scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
}

if (-not $exeFileName) {
    $candidates = @(Get-ChildItem -Path $scriptDir -Filter "*.exe" -File -ErrorAction SilentlyContinue)
    if ($candidates.Count -eq 0) {
        Write-Host "错误：目录里没有找到可执行文件 → $scriptDir"
        exit 1
    }
    if ($candidates.Count -gt 1) {
        Write-Host '错误：目录里有多个可执行文件，请在本脚本顶部显式指定 $exeFileName：'
        $candidates | ForEach-Object { Write-Host ("  - " + $_.Name) }
        exit 1
    }
    $exeFileName = $candidates[0].Name
}

$fullExePath = Join-Path -Path $scriptDir -ChildPath $exeFileName

if (-not (Test-Path -LiteralPath $fullExePath)) {
    Write-Host "错误：找不到可执行文件 → $fullExePath"
    exit 1
}

# 编译时已内置 -room / -port / -silent 参数，此处不传参即可静默运行。
# 如需临时覆盖参数（如 "-port 8080 -silent=false" 以便查看控制台输出），修改 $runArgs。
# 优先级：命令行参数 > config.json > ldflags 注入值 > 代码内置值
$runArgs = ""

# ---- 2) 在跑就停，没跑就起 ----
$procName = $exeFileName -replace '\.exe$', ''
$targetProc = Get-Process -Name $procName -ErrorAction SilentlyContinue

if ($targetProc) {
    Write-Host "正在终止进程 $exeFileName"
    $targetProc | Stop-Process -Force
    Write-Host "进程关闭成功"
}
else {
    Write-Host "后台静默启动服务，无窗口显示"
    $wsShell = New-Object -ComObject WScript.Shell
    # 第二个参数 0 = 完全隐藏窗口，$false = 不等待程序结束
    $wsShell.Run("`"$fullExePath`" $runArgs", 0, $false)
}
