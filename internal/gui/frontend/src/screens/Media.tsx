import { useMemo, useState } from "react"
import {
  RiAddLine,
  RiEditLine,
  RiDeleteBinLine,
  RiFolderOpenLine,
} from "@remixicon/react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  Field,
  FieldLabel,
  FieldGroup,
  FieldError,
  FieldDescription,
} from "@/components/ui/field"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { useSim } from "@/store/simulator"
import * as App from "@/lib/wails/wailsjs/go/gui/App"
import { config as cfgNs } from "@/lib/wails/wailsjs/go/models"

type Profile = cfgNs.ProfileConfig

type FormState = {
  name: string
  token: string
  mediaFilePath: string
  rtsp: string
  encoding: string
  width: string
  height: string
  fps: string
  bitrate: string
  gopLength: string
  snapshotURI: string
  videoSourceToken: string
}

const EMPTY_FORM: FormState = {
  name: "",
  token: "",
  mediaFilePath: "",
  rtsp: "",
  encoding: "",
  width: "",
  height: "",
  fps: "",
  bitrate: "",
  gopLength: "",
  snapshotURI: "",
  videoSourceToken: "",
}

function toFormState(p: Profile): FormState {
  return {
    name: p.name ?? "",
    token: p.token ?? "",
    mediaFilePath: p.media_file_path ?? "",
    rtsp: p.rtsp ?? "",
    encoding: p.encoding ?? "",
    width: p.width ? String(p.width) : "",
    height: p.height ? String(p.height) : "",
    fps: p.fps ? String(p.fps) : "",
    bitrate: p.bitrate ? String(p.bitrate) : "",
    gopLength: p.gop_length ? String(p.gop_length) : "",
    snapshotURI: p.snapshot_uri ?? "",
    videoSourceToken: p.video_source_token ?? "",
  }
}

function formToProfile(f: FormState): Profile {
  return cfgNs.ProfileConfig.createFrom({
    name: f.name,
    token: f.token,
    media_file_path: f.mediaFilePath,
    rtsp: f.rtsp,
    encoding: f.encoding,
    width: Number(f.width || 0),
    height: Number(f.height || 0),
    fps: Number(f.fps || 0),
    bitrate: f.bitrate ? Number(f.bitrate) : 0,
    gop_length: f.gopLength ? Number(f.gopLength) : 0,
    snapshot_uri: f.snapshotURI,
    video_source_token: f.videoSourceToken,
  })
}

export function MediaScreen() {
  const config = useSim((s) => s.config)
  const profiles = useMemo(() => config?.media?.profiles ?? [], [config])

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Profile | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)

  const videoSources = useMemo(() => {
    const seen = new Set<string>()
    for (const p of profiles) {
      const t = p.video_source_token || "VS_DEFAULT"
      seen.add(t)
    }
    return Array.from(seen)
  }, [profiles])

  function openAdd() {
    setEditing(null)
    setDialogOpen(true)
  }
  function openEdit(p: Profile) {
    setEditing(p)
    setDialogOpen(true)
  }

  async function doDelete(token: string) {
    try {
      await App.RemoveProfile(token)
      toast.success(`Profile "${token}" deleted`)
      await useSim.getState().refreshConfig()
    } catch (e) {
      toast.error(String(e))
    } finally {
      setConfirmDelete(null)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Media</h1>
        <Button onClick={openAdd}>
          <RiAddLine data-icon="inline-start" />
          Add profile
        </Button>
      </div>

      {profiles.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No profiles yet</EmptyTitle>
            <EmptyDescription>
              Add one to make the simulator answer Media traffic.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Token</TableHead>
              <TableHead>Media file</TableHead>
              <TableHead>Encoding</TableHead>
              <TableHead>Resolution</TableHead>
              <TableHead>Snapshot URI</TableHead>
              <TableHead className="w-24 text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {profiles.map((p) => (
              <TableRow key={p.token}>
                <TableCell>{p.name}</TableCell>
                <TableCell className="font-mono text-xs">{p.token}</TableCell>
                <TableCell className="max-w-[260px] truncate font-mono text-xs">
                  {p.media_file_path || (
                    <span className="text-muted-foreground">— not set</span>
                  )}
                </TableCell>
                <TableCell>{p.encoding || "—"}</TableCell>
                <TableCell className="font-mono text-xs">
                  {p.width && p.height && p.fps
                    ? `${p.width}x${p.height}@${p.fps}`
                    : "auto"}
                </TableCell>
                <TableCell className="max-w-[200px] truncate font-mono text-xs">
                  {p.snapshot_uri || "—"}
                </TableCell>
                <TableCell className="space-x-1 text-right">
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label="Edit"
                    onClick={() => openEdit(p)}
                  >
                    <RiEditLine />
                  </Button>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label="Delete"
                    onClick={() => setConfirmDelete(p.token ?? null)}
                  >
                    <RiDeleteBinLine />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Video sources</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {videoSources.length === 0 ? (
            <span className="text-sm text-muted-foreground">None.</span>
          ) : (
            videoSources.map((t) => (
              <Badge key={t} variant="secondary" className="font-mono">
                {t}
              </Badge>
            ))
          )}
        </CardContent>
      </Card>

      <ProfileDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        initial={editing}
      />
      <Dialog
        open={confirmDelete !== null}
        onOpenChange={(v) => !v && setConfirmDelete(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete profile</DialogTitle>
            <DialogDescription>
              Remove <code className="font-mono">{confirmDelete}</code>?
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (confirmDelete) void doDelete(confirmDelete)
              }}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function ProfileDialog({
  open,
  onOpenChange,
  initial,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  initial: Profile | null
}) {
  const isEdit = initial !== null
  const [form, setForm] = useState<FormState>(
    initial ? toFormState(initial) : EMPTY_FORM
  )
  const [error, setError] = useState<string | null>(null)

  // Reset form when dialog opens for a different profile.
  const [lastInitial, setLastInitial] = useState<Profile | null>(initial)
  if (lastInitial !== initial) {
    setLastInitial(initial)
    setForm(initial ? toFormState(initial) : EMPTY_FORM)
    setError(null)
  }

  function update<K extends keyof FormState>(k: K, v: string) {
    setForm((f) => ({ ...f, [k]: v }))
  }

  async function pickFile() {
    setError(null)
    try {
      const path = await App.PickMediaFile()
      if (path) update("mediaFilePath", path)
    } catch (e) {
      setError(String(e))
    }
  }

  async function save() {
    setError(null)
    const p = formToProfile(form)
    try {
      if (isEdit) {
        await App.SetProfileMediaFile(p.token ?? "", form.mediaFilePath)
        if (form.snapshotURI || initial?.snapshot_uri) {
          await App.SetProfileSnapshotURI(p.token ?? "", form.snapshotURI)
        }
        toast.success(`Profile "${p.token}" updated`)
      } else {
        await App.AddProfile(p)
        toast.success(`Profile "${p.token}" added`)
      }
      await useSim.getState().refreshConfig()
      onOpenChange(false)
    } catch (e) {
      setError(String(e))
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit profile" : "Add profile"}</DialogTitle>
          <DialogDescription>
            The simulator hosts the RTSP stream itself by looping the local
            mp4 you select. GetStreamUri returns a URL pointing at this
            simulator; codec, resolution, and frame rate are auto-detected
            from the file.
          </DialogDescription>
        </DialogHeader>

        <FieldGroup>
          <div className="grid grid-cols-2 gap-4">
            <Field>
              <FieldLabel>Name</FieldLabel>
              <Input
                value={form.name}
                onChange={(e) => update("name", e.target.value)}
                disabled={isEdit}
              />
            </Field>
            <Field>
              <FieldLabel>Token</FieldLabel>
              <Input
                value={form.token}
                onChange={(e) => update("token", e.target.value)}
                disabled={isEdit}
                className="font-mono"
              />
            </Field>
          </div>

          <Field>
            <FieldLabel>Media file</FieldLabel>
            <div className="flex gap-2">
              <Input
                value={form.mediaFilePath}
                onChange={(e) => update("mediaFilePath", e.target.value)}
                placeholder="/absolute/path/to/video.mp4"
                className="font-mono"
                aria-label="Media file path"
              />
              <Button
                type="button"
                variant="outline"
                onClick={() => void pickFile()}
                aria-label="Browse for media file"
              >
                <RiFolderOpenLine data-icon="inline-start" />
                Browse
              </Button>
            </div>
            <FieldDescription>
              Required. The embedded RTSP server loops this file forever.
            </FieldDescription>
          </Field>

          <div className="grid grid-cols-3 gap-4">
            <Field>
              <FieldLabel>Encoding</FieldLabel>
              <Input
                value={form.encoding || "auto"}
                disabled
                className="font-mono"
              />
              <FieldDescription>Detected on Start.</FieldDescription>
            </Field>
            <Field>
              <FieldLabel>Resolution</FieldLabel>
              <Input
                value={
                  form.width && form.height
                    ? `${form.width}x${form.height}`
                    : "auto"
                }
                disabled
                className="font-mono"
              />
              <FieldDescription>Detected on Start.</FieldDescription>
            </Field>
            <Field>
              <FieldLabel>FPS</FieldLabel>
              <Input
                value={form.fps || "auto"}
                disabled
                className="font-mono"
              />
              <FieldDescription>Detected on Start.</FieldDescription>
            </Field>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <Field>
              <FieldLabel>Snapshot URI (optional)</FieldLabel>
              <Input
                value={form.snapshotURI}
                onChange={(e) => update("snapshotURI", e.target.value)}
                placeholder="http(s)://..."
                className="font-mono"
              />
            </Field>
            <Field>
              <FieldLabel>Video source token</FieldLabel>
              <Input
                value={form.videoSourceToken}
                onChange={(e) => update("videoSourceToken", e.target.value)}
                className="font-mono"
              />
            </Field>
          </div>

          {error && (
            <Field data-invalid>
              <FieldError>{error}</FieldError>
            </Field>
          )}
        </FieldGroup>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => void save()}>Save</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
