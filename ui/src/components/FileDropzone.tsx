'use client'

import { useCallback, useEffect, useState, useRef } from 'react'
import { extractQMDsFromZip, extractQMDsFromTarGz, extractQMDsFromFolder, sortFilesLexicographically } from '@/lib/fileUtils'

interface FileDropzoneProps {
  onFileSelected: (file: File) => void
  onFilesSelected: (files: File[]) => void
  disabled?: boolean
  onError?: (message: string) => void
  multiple?: boolean
  existingFiles?: File[]
}

const ACCEPT_CONFIG = {
  'text/plain': ['.qmd'],
  'application/zip': ['.zip'],
  'application/x-gzip': ['.tar.gz', '.gz'],
}

export function FileDropzone({
  onFileSelected,
  onFilesSelected,
  disabled = false,
  onError,
  multiple = false,
  existingFiles = [],
}: FileDropzoneProps) {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [dragActive, setDragActive] = useState(false)

  const processFiles = useCallback(
    async (files: File[]) => {
      const allQMDFiles: File[] = []

      for (const file of files) {
        if (file.name.toLowerCase().endsWith('.zip')) {
          try {
            const extracted = await extractQMDsFromZip(file)
            allQMDFiles.push(...extracted)
          } catch (err) {
            if (onError) {
              onError(`Failed to extract ${file.name}: ${err}`)
            }
          }
        } else if (file.name.toLowerCase().endsWith('.tar.gz') || file.name.toLowerCase().endsWith('.tgz')) {
          try {
            const extracted = await extractQMDsFromTarGz(file)
            allQMDFiles.push(...extracted)
          } catch (err) {
            if (onError) {
              onError(`Failed to extract ${file.name}: ${err}`)
            }
          }
        } else if (file.name.toLowerCase().endsWith('.qmd')) {
          allQMDFiles.push(file)
        } else {
          if (onError) {
            onError(`Unsupported file type: ${file.name}`)
          }
        }
      }

      if (allQMDFiles.length === 0) {
        if (onError) {
          onError('No .qmd files found')
        }
        return
      }

      const sortedFiles = sortFilesLexicographically(allQMDFiles)

      if (multiple) {
        const combinedFiles = sortFilesLexicographically([...existingFiles, ...sortedFiles])
        onFilesSelected(combinedFiles)
      } else {
        onFileSelected(sortedFiles[0])
      }
    },
    [onFileSelected, onFilesSelected, onError, multiple, existingFiles]
  )

  useEffect(() => {
    if (disabled) return

    let counter = 0

    function handleDragEnter(e: DragEvent) {
      if (Array.from(e.dataTransfer?.types || []).includes('Files')) {
        counter++
        setDragActive(true)
      }
    }

    function handleDragLeave() {
      counter = Math.max(counter - 1, 0)
      if (counter === 0) setDragActive(false)
    }

    function handleDragOver(e: DragEvent) {
      e.preventDefault()
    }

    async function handleDrop(e: DragEvent) {
      e.preventDefault()
      counter = 0
      setDragActive(false)

      const items = Array.from(e.dataTransfer?.items || [])
      const allFiles: File[] = []

      if (items.length > 0 && typeof items[0].webkitGetAsEntry === 'function') {
        for (const item of items) {
          const entry = item.webkitGetAsEntry()
          if (entry) {
            if (entry.isFile) {
              const file = item.getAsFile()
              if (file) allFiles.push(file)
            } else if (entry.isDirectory) {
              try {
                const extractedFiles = await extractQMDsFromFolder(entry)
                allFiles.push(...extractedFiles)
              } catch (err) {
                console.error('Failed to extract from folder:', err)
              }
            }
          }
        }
      } else {
        const files = Array.from(e.dataTransfer?.files || [])
        allFiles.push(...files)
      }

      if (allFiles.length > 0) {
        console.log('Extracted files:', allFiles.map(f => f.name))  // DEBUG
        processFiles(allFiles)
      }
    }

    window.addEventListener('dragenter', handleDragEnter)
    window.addEventListener('dragleave', handleDragLeave)
    window.addEventListener('dragover', handleDragOver)
    window.addEventListener('drop', handleDrop)

    return () => {
      window.removeEventListener('dragenter', handleDragEnter)
      window.removeEventListener('dragleave', handleDragLeave)
      window.removeEventListener('dragover', handleDragOver)
      window.removeEventListener('drop', handleDrop)
    }
  }, [disabled, processFiles, multiple])

  const handleClick = () => {
    if (!disabled && fileInputRef.current) {
      fileInputRef.current.click()
    }
  }

  const handleFileInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || [])
    if (files.length > 0) {
      processFiles(files)
    }
    e.target.value = ''
  }

  return (
    <div
      onClick={handleClick}
      className={
        'border-2 border-dashed rounded-lg p-6 text-center cursor-pointer transition-colors ' +
        (disabled
          ? 'opacity-50 cursor-not-allowed border-input'
          : dragActive
          ? 'border-primary bg-muted text-foreground'
          : 'border-input hover:border-primary text-muted-foreground')
      }
    >
      <input
        ref={fileInputRef}
        type="file"
        multiple={multiple}
        accept={Object.values(ACCEPT_CONFIG).flat().join(',')}
        onChange={handleFileInputChange}
        className="hidden"
        disabled={disabled}
      />
      <p className="text-sm">
        {multiple
          ? 'Click or drag and drop .qmd files, folders, or ZIP archives here'
          : 'Click or drag and drop a .qmd file here to compare against available hashtables'
        }
      </p>
    </div>
  )
}
