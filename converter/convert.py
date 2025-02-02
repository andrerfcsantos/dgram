import argparse
import json
from pathlib import Path
import glob

from deepgram_captions import DeepgramConverter, srt

def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument('file_globs', nargs='+', help='globs of files to process')
    return parser.parse_args()

def main():
    args = parse_arguments()

    files = []
    for file_glob in args.file_globs:
        files.extend(glob.glob(file_glob))

    for file in files:
        file_data = ""

        with open(file, encoding="utf-8") as f:
            file_data = f.read()

        dg_response = json.loads(file_data)

        transcription = DeepgramConverter(dg_response)
        captions = srt(transcription)

        file = file.replace("_response", "")

        output_file = Path(file).with_suffix('.srt')

        with open(output_file, 'w+', encoding="utf-8") as f:
            f.write(captions)



if __name__ == '__main__':
    main()
