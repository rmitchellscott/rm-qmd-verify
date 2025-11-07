import { useState, useEffect } from 'react'
import { BrowserRouter, Routes, Route, useNavigate } from 'react-router-dom'
import { ThemeProvider } from 'next-themes'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Toaster } from '@/components/ui/sonner'
import { toast } from 'sonner'
import ThemeSwitcher from '@/components/ThemeSwitcher'
import { FileDropzone } from '@/components/FileDropzone'
import { FileList } from '@/components/FileList'
import { CompatibilityMatrix, type CompareResponse } from '@/components/CompatibilityMatrix'
import { FileDetailModal } from '@/components/FileDetailModal'
import { ComparisonResultsPage } from '@/components/ComparisonResultsPage'
import { waitForJobWS } from '@/lib/websocket'
import type { JobStatus } from '@/lib/websocket'

interface FileStatus {
  status: 'pending' | 'uploading' | 'processing' | 'success' | 'error';
  progress: number;
  message?: string;
}

function HomePage() {
  const navigate = useNavigate()
  const [files, setFiles] = useState<File[]>([])
  const [fileStatuses, setFileStatuses] = useState<Map<string, FileStatus>>(new Map())
  const [fileResults, setFileResults] = useState<Map<string, CompareResponse>>(new Map())
  const [selectedFileForModal, setSelectedFileForModal] = useState<string | null>(null)
  const [isProcessing, setIsProcessing] = useState(false)
  const [shouldNavigateToResults, setShouldNavigateToResults] = useState(false)
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

    const refreshHashtables = async () => {
      try {
        // Trigger hashtable check/reload on page load
        await fetch('/api/hashtables')
      } catch (error) {
        console.error('Failed to refresh hashtables:', error)
      }
    }

    const refreshTrees = async () => {
      try {
        // Trigger tree check/reload on page load
        await fetch('/api/trees')
      } catch (error) {
        console.error('Failed to refresh trees:', error)
      }
    }

    fetchVersionInfo()
    refreshHashtables()
    refreshTrees()
  }, [])

  useEffect(() => {
    const allFilesProcessed = files.length > 0 && files.every(f => {
      const status = fileStatuses.get(f.name)
      return status && (status.status === 'success' || status.status === 'error')
    })

    if (shouldNavigateToResults && !isProcessing && allFilesProcessed && files.length > 1) {
      navigate('/results', {
        state: {
          results: Object.fromEntries(fileResults),
          filenames: files.map(f => f.name)
        }
      })
      setShouldNavigateToResults(false)
    }
  }, [shouldNavigateToResults, isProcessing, fileStatuses, fileResults, files, navigate])

  const handleFilesSelected = (selectedFiles: File[]) => {
    setFiles(selectedFiles)
    const newStatuses = new Map<string, FileStatus>()
    selectedFiles.forEach(file => {
      newStatuses.set(file.name, {
        status: 'pending',
        progress: 0
      })
    })
    setFileStatuses(newStatuses)
    setFileResults(new Map())
  }

  const handleError = (message: string) => {
    toast.error("Error", {
      description: message,
    })
  }

  const updateFileStatus = (filename: string, updates: Partial<FileStatus>) => {
    setFileStatuses(prev => {
      const newMap = new Map(prev)
      const existing = newMap.get(filename)
      if (existing) {
        newMap.set(filename, { ...existing, ...updates })
      }
      return newMap
    })
  }

  const uploadBatchWithProgress = async (
    files: File[],
    onProgress: (percent: number) => void
  ): Promise<{ jobId: string }> => {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest()

      xhr.upload.addEventListener('progress', (event) => {
        if (event.lengthComputable) {
          const percentComplete = (event.loaded / event.total) * 100
          onProgress(percentComplete)
        }
      })

      xhr.addEventListener('load', () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const response = JSON.parse(xhr.responseText)
            resolve(response)
          } catch (err) {
            reject(new Error('Invalid response format'))
          }
        } else {
          reject(new Error(xhr.responseText || `HTTP ${xhr.status}`))
        }
      })

      xhr.addEventListener('error', () => {
        reject(new Error('Upload failed'))
      })

      const formData = new FormData()
      files.forEach(file => {
        formData.append('files', file)
      })

      xhr.open('POST', '/api/compare')
      xhr.send(formData)
    })
  }

  const handleUploadAll = async () => {
    if (files.length === 0) return

    setIsProcessing(true)
    setShouldNavigateToResults(false)

    try {
      // Set all files to uploading state
      files.forEach(file => {
        updateFileStatus(file.name, { status: 'uploading', progress: 0 })
      })

      // Upload all files in a single batch request
      const { jobId } = await uploadBatchWithProgress(files, (progress) => {
        // Update all files with upload progress
        files.forEach(file => {
          updateFileStatus(file.name, { progress, message: 'Uploading...' })
        })
      })

      // Set all files to processing state
      files.forEach(file => {
        updateFileStatus(file.name, {
          status: 'processing',
          progress: 0,
          message: 'Processing...'
        })
      })

      // Wait for batch processing to complete
      await waitForJobWS(jobId, (status: JobStatus) => {
        // Update all files with processing progress
        files.forEach(file => {
          updateFileStatus(file.name, {
            progress: status.progress,
            message: status.message
          })
        })
      })

      // Fetch batch results
      const resultsResponse = await fetch(`/api/results/${jobId}`)
      if (!resultsResponse.ok) {
        throw new Error('Failed to fetch results')
      }

      const batchResults = await resultsResponse.json()

      // Handle single file (backward compatibility)
      if (files.length === 1) {
        const results: CompareResponse = batchResults
        setFileResults(prev => new Map(prev).set(files[0].name, results))

        updateFileStatus(files[0].name, {
          status: 'success',
          progress: 100,
          message: 'Complete'
        })
      } else {
        // Handle multiple files (batch response)
        const resultsMap: Record<string, CompareResponse> = batchResults
        const newResults = new Map(fileResults)

        files.forEach(file => {
          if (resultsMap[file.name]) {
            newResults.set(file.name, resultsMap[file.name])
            updateFileStatus(file.name, {
              status: 'success',
              progress: 100,
              message: 'Complete'
            })
          } else {
            updateFileStatus(file.name, {
              status: 'error',
              message: 'No results received'
            })
          }
        })

        setFileResults(newResults)
      }
    } catch (error) {
      // Mark all files as error
      files.forEach(file => {
        updateFileStatus(file.name, {
          status: 'error',
          message: error instanceof Error ? error.message : 'An error occurred'
        })
      })

      toast.error("Processing failed", {
        description: error instanceof Error ? error.message : 'Unknown error',
      })
    } finally {
      setIsProcessing(false)

      if (files.length > 1) {
        setShouldNavigateToResults(true)
      }
    }
  }

  const handleReset = () => {
    setFiles([])
    setFileStatuses(new Map())
    setFileResults(new Map())
    setSelectedFileForModal(null)
  }

  const handleRemoveFile = (index: number) => {
    const newFiles = files.filter((_, i) => i !== index)
    const removedFileName = files[index].name

    setFiles(newFiles)
    setFileStatuses(prev => {
      const newMap = new Map(prev)
      newMap.delete(removedFileName)
      return newMap
    })
    setFileResults(prev => {
      const newMap = new Map(prev)
      newMap.delete(removedFileName)
      return newMap
    })
  }

  const hasResults = fileResults.size > 0
  const allFilesProcessed = files.length > 0 && files.every(f => {
    const status = fileStatuses.get(f.name)
    return status && (status.status === 'success' || status.status === 'error')
  })

  const completedFiles = Array.from(fileStatuses.values())
    .filter(s => s.status === 'success' || s.status === 'error').length
  const overallProgress = files.length > 0 ? (completedFiles / files.length) * 100 : 0

  return (
    <div className="min-h-screen flex flex-col">
      <header className="flex items-center justify-between px-8 py-2 bg-background">
        <h1 className="text-2xl font-bold">reMarkable QMD Verifier</h1>
        <ThemeSwitcher />
      </header>
      <main className="flex-1 bg-background pt-0 pb-8 px-8">
        <div className="max-w-md mx-auto space-y-6">
          <Card className="bg-card">
            <CardHeader>
              <CardTitle>Verify QMD Files</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <FileDropzone
                onFileSelected={(file) => handleFilesSelected([file])}
                onFilesSelected={handleFilesSelected}
                disabled={isProcessing}
                onError={handleError}
                multiple={true}
              />

              {files.length > 0 && (
                <FileList
                  files={files}
                  onRemove={handleRemoveFile}
                  disabled={isProcessing}
                />
              )}

              <div className="flex gap-2">
                <Button
                  variant="outline"
                  onClick={handleReset}
                  className="flex-1"
                  disabled={files.length === 0 || isProcessing}
                >
                  Reset
                </Button>
                <Button
                  onClick={handleUploadAll}
                  disabled={files.length === 0 || isProcessing}
                  className="flex-1"
                >
                  {isProcessing ? (
                    <>
                      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      Processing...
                    </>
                  ) : (
                    'Compare'
                  )}
                </Button>
              </div>

              {isProcessing && files.length > 0 && overallProgress > 0 && overallProgress < 100 && (
                <Progress value={overallProgress} />
              )}
            </CardContent>
          </Card>
        </div>

        {hasResults && allFilesProcessed && files.length === 1 && fileResults.has(files[0].name) && (
          <div className="max-w-4xl mx-auto mt-6">
            <Card className="bg-card">
              <CardHeader>
                <CardTitle>Compatibility Results</CardTitle>
              </CardHeader>
              <CardContent>
                <CompatibilityMatrix
                  results={fileResults.get(files[0].name)!}
                />
              </CardContent>
            </Card>
          </div>
        )}

        <FileDetailModal
          filename={selectedFileForModal}
          results={selectedFileForModal ? fileResults.get(selectedFileForModal) : undefined}
          open={!!selectedFileForModal}
          onOpenChange={(open) => !open && setSelectedFileForModal(null)}
        />
      </main>
      {versionInfo && (
        <footer className="py-2 bg-background">
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
      <Toaster />
    </div>
  )
}

export default function App() {
  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/results" element={<ComparisonResultsPage />} />
        </Routes>
      </BrowserRouter>
    </ThemeProvider>
  )
}
