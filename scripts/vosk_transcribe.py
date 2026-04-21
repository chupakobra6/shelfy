#!/usr/bin/env python3
import json
import sys
import wave

from vosk import KaldiRecognizer, Model, SetLogLevel


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: vosk-transcribe <model-dir> <wav-path>", file=sys.stderr)
        return 2
    model_dir, wav_path = sys.argv[1], sys.argv[2]
    SetLogLevel(-1)
    model = Model(model_dir)
    with wave.open(wav_path, "rb") as wf:
        if wf.getnchannels() != 1 or wf.getsampwidth() != 2:
            raise RuntimeError("unsupported wav format: expected mono 16-bit PCM")
        rec = KaldiRecognizer(model, wf.getframerate())
        parts = []
        while True:
            data = wf.readframes(4000)
            if not data:
                break
            if rec.AcceptWaveform(data):
                parts.append(json.loads(rec.Result()).get("text", ""))
        parts.append(json.loads(rec.FinalResult()).get("text", ""))
    print(" ".join(part for part in parts if part).strip())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
