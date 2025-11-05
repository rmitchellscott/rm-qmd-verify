import { useState, useEffect } from 'react'
import { ThemeProvider } from 'next-themes'
import { CheckCircle2, XCircle, Loader2, ChevronRight, ChevronDown, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Toaster } from '@/components/ui/sonner'
import { toast } from 'sonner'
import ThemeSwitcher from '@/components/ThemeSwitcher'
import { FileDropzone } from '@/components/FileDropzone'

interface ComparisonResult {
  hashtable: string
  os_version: string
  device: string
  compatible: boolean
  error_detail?: string
  missing_hashes?: string[]
}

const deviceNames: Record<string, { short: string; full: string }> = {
  'rm1': { short: 'rM1', full: 'reMarkable 1' },
  'rm2': { short: 'rM2', full: 'reMarkable 2' },
  'rmpp': { short: 'rMPP', full: 'Paper Pro' },
  'rmppm': { short: 'rMPPM', full: 'Paper Pro Move' },
}

interface CompareResponse {
  compatible: ComparisonResult[]
  incompatible: ComparisonResult[]
  total_checked: number
}

interface VersionInfo {
  full: string
  majorMinorPatch: string
  build: string | null
  parts: number[]
}

function parseVersion(version: string): VersionInfo {
  const parts = version.split('.').map(p => parseInt(p, 10))
  const build = parts.length > 3 ? parts[3].toString() : null
  const majorMinorPatch = parts.slice(0, 3).join('.')

  return {
    full: version,
    majorMinorPatch,
    build,
    parts
  }
}

function compareVersions(a: string, b: string): number {
  const aParts = parseVersion(a).parts
  const bParts = parseVersion(b).parts

  for (let i = 0; i < Math.max(aParts.length, bParts.length); i++) {
    const aVal = aParts[i] || 0
    const bVal = bParts[i] || 0
    if (aVal !== bVal) {
      return bVal - aVal
    }
  }
  return 0
}

function AppContent() {
  const [file, setFile] = useState<File | null>(null)
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState<CompareResponse | null>(null)
  const [expandedVersions, setExpandedVersions] = useState<Set<string>>(new Set())
  const [versionInfo, setVersionInfo] = useState<{ version: string } | null>(null)

  useEffect(() => {
    const fetchVersionInfo = async () => {
      try {
        const response = await fetch('/api/version')
        if (response.ok) {
          const data = await response.json()
          setVersionInfo(data)
        }
      } catch (error) {
        console.error('Failed to fetch version info:', error)
      }
    }
    fetchVersionInfo()
  }, [])

  const handleFileSelected = (selectedFile: File) => {
    setFile(selectedFile)
  }

  const handleError = (message: string) => {
    toast.error("Invalid file type", {
      description: message,
    })
  }

  const handleUpload = async () => {
    if (!file) {
      return
    }

    setLoading(true)
    setResults(null)

    try {
      const formData = new FormData()
      formData.append('file', file)

      const response = await fetch('/api/compare', {
        method: 'POST',
        body: formData,
      })

      if (!response.ok) {
        const errorData = await response.json()
        throw new Error(errorData.error || 'Upload failed')
      }

      const data: CompareResponse = await response.json()
      setResults(data)
    } catch (error) {
      toast.error("Upload failed", {
        description: error instanceof Error ? error.message : "An error occurred",
      })
    } finally {
      setLoading(false)
    }
  }

  const handleReset = () => {
    setFile(null)
    setResults(null)
  }

  const toggleVersionExpansion = (majorMinorPatch: string) => {
    setExpandedVersions(prev => {
      const next = new Set(prev)
      if (next.has(majorMinorPatch)) {
        next.delete(majorMinorPatch)
      } else {
        next.add(majorMinorPatch)
      }
      return next
    })
  }

  const buildCompatibilityMatrix = () => {
    if (!results) return { versionGroups: [], deviceKeys: [], matrix: {} }

    const allResults = [...results.compatible, ...results.incompatible]
    const deviceKeys = ['rm1', 'rm2', 'rmpp', 'rmppm']

    const matrix: Record<string, Record<string, ComparisonResult | null>> = {}
    allResults.forEach(result => {
      if (!matrix[result.os_version]) {
        matrix[result.os_version] = {}
      }
      matrix[result.os_version][result.device] = result
    })

    const versionsByGroup = new Map<string, string[]>()
    allResults.forEach(result => {
      const versionInfo = parseVersion(result.os_version)
      const group = versionInfo.majorMinorPatch
      if (!versionsByGroup.has(group)) {
        versionsByGroup.set(group, [])
      }
      const versions = versionsByGroup.get(group)!
      if (!versions.includes(result.os_version)) {
        versions.push(result.os_version)
      }
    })

    versionsByGroup.forEach((versions, _group) => {
      versions.sort(compareVersions)
    })

    const sortedGroups = Array.from(versionsByGroup.keys()).sort((a, b) => {
      const aMaxVersion = versionsByGroup.get(a)![0]
      const bMaxVersion = versionsByGroup.get(b)![0]
      return compareVersions(aMaxVersion, bMaxVersion)
    })

    const versionGroups = sortedGroups.map(group => ({
      majorMinorPatch: group,
      versions: versionsByGroup.get(group)!,
      hasMultipleBuilds: versionsByGroup.get(group)!.length > 1
    }))

    return { versionGroups, deviceKeys, matrix }
  }

  return (
    <>
      <header className="flex items-center justify-between px-8 py-2 bg-background">
        <h1 className="text-2xl font-bold">reMarkable QMD Verifier</h1>
        <ThemeSwitcher />
      </header>
      <main className="bg-background pt-0 pb-8 px-8">
        <div className="max-w-md mx-auto space-y-6">
          <Card className="bg-card">
          <CardHeader>
            <CardTitle>Verify QMD File</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <FileDropzone
                onFileSelected={handleFileSelected}
                onFilesSelected={() => {}}
                disabled={loading}
                onError={handleError}
                multiple={false}
              />
              {file && (
                <div className="mt-4 p-3 bg-muted rounded-md">
                  <p className="text-sm font-medium">{file.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {(file.size / 1024).toFixed(2)} KB
                  </p>
                </div>
              )}
            </div>

            <div className="flex gap-2">
              <Button variant="outline" onClick={handleReset} className="flex-1" disabled={!file && !results}>
                Reset
              </Button>
              <Button
                onClick={handleUpload}
                disabled={!file || loading}
                className="flex-1"
              >
                {loading ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Comparing...
                  </>
                ) : (
                  'Compare'
                )}
              </Button>
            </div>
          </CardContent>
        </Card>
        </div>

        {results && (() => {
          const { versionGroups, deviceKeys, matrix } = buildCompatibilityMatrix()

          const renderCompatibilityCell = (result: ComparisonResult | null | undefined, device: string) => (
            <TableCell key={device} className="text-center">
              {result?.compatible === true && (
                <Tooltip>
                  <TooltipTrigger>
                    <CheckCircle2 className="h-5 w-5 text-green-600 inline-block" />
                  </TooltipTrigger>
                  <TooltipContent>
                    Compatible
                  </TooltipContent>
                </Tooltip>
              )}
              {result?.compatible === false && (
                <Popover>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <PopoverTrigger asChild>
                        <button className="cursor-pointer border-none bg-transparent p-0">
                          <XCircle className="h-5 w-5 text-red-600" />
                        </button>
                      </PopoverTrigger>
                    </TooltipTrigger>
                    <TooltipContent>Click for details</TooltipContent>
                  </Tooltip>
                  <PopoverContent>
                    <div className="text-sm">
                      <div className="mb-2">Missing {result.missing_hashes && result.missing_hashes.length > 1 ? 'hashes' : 'hash'}:</div>
                      {result.missing_hashes && result.missing_hashes.map(hash => (
                        <div key={hash} className="font-mono">{hash}</div>
                      ))}
                      {(!result.missing_hashes || result.missing_hashes.length === 0) && (
                        <div className="font-mono">Unknown</div>
                      )}
                    </div>
                  </PopoverContent>
                </Popover>
              )}
              {!result && (
                <span className="text-muted-foreground">—</span>
              )}
            </TableCell>
          )

          return (
            <div className="max-w-4xl mx-auto mt-6">
              <Card className="bg-card">
                <CardHeader>
                  <CardTitle>Compatibility Results</CardTitle>
                </CardHeader>
                <CardContent>
                  <TooltipProvider>
                    <Table>
                      <TableHeader>
                      <TableRow>
                        <TableHead className="min-w-24 sm:min-w-32"></TableHead>
                        {deviceKeys.map(device => (
                          <TableHead key={device} className="text-center">
                            <span className="sm:hidden">{deviceNames[device].short}</span>
                            <span className="hidden sm:inline">{deviceNames[device].full}</span>
                          </TableHead>
                        ))}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {versionGroups.map(group => {
                        const isExpanded = expandedVersions.has(group.majorMinorPatch)

                        if (group.hasMultipleBuilds) {
                          return (
                            <>
                              <TableRow key={group.majorMinorPatch} className="cursor-pointer hover:bg-muted/50">
                                <TableCell
                                  className="font-medium"
                                  onClick={() => toggleVersionExpansion(group.majorMinorPatch)}
                                >
                                  <div className="flex items-center gap-2">
                                    <span>{group.majorMinorPatch}</span>
                                    {isExpanded ? (
                                      <ChevronDown className="h-4 w-4" />
                                    ) : (
                                      <ChevronRight className="h-4 w-4" />
                                    )}
                                  </div>
                                </TableCell>
                                {isExpanded ? (
                                  deviceKeys.map(device => (
                                    <TableCell key={device} className="text-center"></TableCell>
                                  ))
                                ) : (
                                  deviceKeys.map(device => {
                                    const allResults = group.versions.map(v => matrix[v]?.[device]).filter(Boolean)

                                    if (allResults.length === 0) {
                                      return <TableCell key={device} className="text-center">
                                        <span className="text-muted-foreground">—</span>
                                      </TableCell>
                                    }

                                    const hasFailure = allResults.some(r => r?.compatible === false)
                                    const hasSuccess = allResults.some(r => r?.compatible === true)

                                    if (hasFailure && hasSuccess) {
                                      return (
                                        <TableCell key={device} className="text-center">
                                          <Tooltip>
                                            <TooltipTrigger asChild>
                                              <button
                                                className="cursor-pointer border-none bg-transparent p-0"
                                                onClick={() => toggleVersionExpansion(group.majorMinorPatch)}
                                              >
                                                <AlertCircle className="h-5 w-5 text-yellow-600 inline-block" />
                                              </button>
                                            </TooltipTrigger>
                                            <TooltipContent>
                                              Mixed results - click to see details
                                            </TooltipContent>
                                          </Tooltip>
                                        </TableCell>
                                      )
                                    } else if (hasFailure) {
                                      const failureResult = allResults.find(r => r?.compatible === false)
                                      return renderCompatibilityCell(failureResult!, device)
                                    } else {
                                      const successResult = allResults.find(r => r?.compatible === true)
                                      return renderCompatibilityCell(successResult!, device)
                                    }
                                  })
                                )}
                              </TableRow>
                              {isExpanded && group.versions.map(version => (
                                <TableRow key={version} className="bg-muted/20">
                                  <TableCell className="font-medium pl-10">
                                    {version}
                                  </TableCell>
                                  {deviceKeys.map(device => {
                                    const result = matrix[version]?.[device]
                                    return renderCompatibilityCell(result, device)
                                  })}
                                </TableRow>
                              ))}
                            </>
                          )
                        } else {
                          const version = group.versions[0]
                          return (
                            <TableRow key={version}>
                              <TableCell className="font-medium">{version}</TableCell>
                              {deviceKeys.map(device => {
                                const result = matrix[version]?.[device]
                                return renderCompatibilityCell(result, device)
                              })}
                            </TableRow>
                          )
                        }
                      })}
                    </TableBody>
                    </Table>
                  </TooltipProvider>
                </CardContent>
              </Card>
            </div>
          )
        })()}
      </main>
      <Toaster />
      {versionInfo && (
        <footer className="fixed bottom-0 left-0 right-0 py-2 bg-background">
          <div className="text-center text-sm text-muted-foreground">
            <span>{versionInfo.version} • </span>
            <a
              href="https://github.com/rmitchellscott/rm-qmd-verify"
              target="_blank"
              rel="noopener noreferrer"
              className="text-muted-foreground hover:underline"
            >
              GitHub
            </a>
          </div>
        </footer>
      )}
    </>
  )
}

export default function App() {
  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      <AppContent />
    </ThemeProvider>
  )
}
