import { Heading, Text, VStack } from "@hope-ui/solid"
import { useT } from "~/hooks"
import { Group } from "~/types"
import CommonSettings from "../settings/Common"

const sections = [
  {
    title: "media_settings.execution",
    keys: [
      "transcode_enabled",
      "transcode_run_mode",
      "transcode_hwaccel",
      "transcode_ffmpeg_path",
      "transcode_ffprobe_path",
      "transcode_worker_secret",
    ],
  },
  {
    title: "media_settings.rules",
    keys: [
      "transcode_min_size_gb",
      "transcode_min_bitrate_mbps",
      "transcode_source_codecs",
      "transcode_source_extensions",
    ],
  },
  {
    title: "media_settings.output",
    keys: [
      "transcode_output_format",
      "transcode_output_codec",
      "transcode_output_bitrate",
      "transcode_output_resolution",
      "transcode_output_audio_codec",
      "transcode_output_audio_bitrate",
    ],
  },
  {
    title: "media_settings.performance",
    keys: [
      "transcode_segment_duration",
      "transcode_local_concurrency",
      "transcode_chunk_duration_sec",
      "transcode_max_chunk_concurrency",
      "transcode_chunk_prefetch",
    ],
  },
  {
    title: "media_settings.cache",
    keys: [
      "transcode_cache_path",
      "transcode_cache_max_gb",
      "transcode_job_timeout_min",
      "transcode_idle_timeout_sec",
      "transcode_chunk_idle_sec",
    ],
  },
]

export default function MediaTranscodePage() {
  const t = useT()
  return (
    <VStack
      class="media-settings-page"
      w="$full"
      alignItems="stretch"
      spacing="$4"
      p={{ "@initial": "$3", "@md": "$6" }}
      maxW="1300px"
    >
      <VStack alignItems="flex-start" spacing="2px">
        <Heading size="lg">{t("manage.sidemenu.media_transcode")}</Heading>
        <Text fontSize="$sm" color="$neutral11">
          {t("media_settings.transcode_desc")}
        </Text>
      </VStack>
      <CommonSettings group={Group.TRANSCODE} sections={sections} />
    </VStack>
  )
}
