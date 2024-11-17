import subprocess
import os
import time
import shutil
import sys
from telegram import Bot, InputFile,InputSticker, Sticker
from telegram.constants import StickerFormat, StickerType
from telegram._utils.types import FileInput, FileLike
from telegram.ext import ApplicationBuilder
from telegram.error import TelegramError
import asyncio

def clear_directory(directory):
    for filename in os.listdir(directory):
        file_path = os.path.join(directory, filename)
        try:
            if os.path.isfile(file_path) or os.path.islink(file_path):
                os.unlink(file_path)
            elif os.path.isdir(file_path):
                shutil.rmtree(file_path)
        except Exception as e:
            print(f'Не удалось удалить {file_path}. Причина: {e}')

def get_video_dimensions(input_video):
    cmd = [
        'ffprobe',
        '-v', 'error',
        '-select_streams', 'v:0',
        '-count_packets',
        '-show_entries', 'stream=width,height',
        '-of', 'csv=p=0',
        input_video
    ]
    output = subprocess.check_output(cmd).decode('utf-8').strip().split(',')
    return int(output[0]), int(output[1])

def process_video(input_video, output_dir, tile_width=100, tile_height=100, background_color="0x000000"):
    video_width, video_height = get_video_dimensions(input_video)
    tiles_x = video_width // tile_width
    tiles_y = video_height // tile_height

    created_files = []

    for j in range(tiles_y):
        for i in range(tiles_x):
            x = i * tile_width
            y = j * tile_height
            output_file = os.path.join(output_dir, f"emoji_{j}_{i}.webm")
            proxy_file = os.path.join(output_dir, f"emoji_proxy_{j}_{i}.webm")

            cmd = [
                'ffmpeg',
                '-i', input_video,
                '-c:v', 'libvpx-vp9',
                # '-vf', f'crop={tile_width}:{tile_height}:{x}:{y},scale=100:100,setsar=1:1',
                # '-vf', f'crop={tile_width}:{tile_height}:{x}:{y},colorkey={background_color}:similarity=0.2:blend=0.1,scale=100:100,setsar=1:1',
                '-vf', f'crop={tile_width}:{tile_height}:{x}:{y},'
                       f'colorkey={background_color}:similarity=0.2:blend=0.1,'
                       # f'scale=100:92,pad=100:100:0:8:color=0x00000000,'
                       f'setsar=1:1',
                '-crf', '24',
                '-b:v', '0',
                '-b:a', '256k',
                '-t', '2.99',
                # '-c:a', 'libopus',
                '-r', '10',
                '-auto-alt-ref', '1',
                '-metadata:s:v:0', 'alpha_mode=1',
                '-an',
                output_file,
            ]

            # cmd = [
            #     'ffmpeg',
            #     '-i', input_video,
            #     '-c:v', 'libvpx-vp9',
            #     '-vf', f'crop={tile_width}:{tile_height}:{x}:{y},'
            #            f'chromakey={background_color}:0.01:0.01,'
            #            f'format=yuva420p,scale=100:100,setsar=1:1',
            #     '-crf', '30',
            #     '-b:v', '0',
            #     '-t', '2.99',
            #     '-c:a', 'libopus',
            #     '-auto-alt-ref', '0',
            #     '-r', '20',
            #     # '-pix_fmt', 'yuva420p',
            #     '-an',
            #     output_file
            # ]

            # subprocess.run(cmd, check=True)

            # cmd = [
            #     'ffmpeg',
            #     '-y',
            #     '-i', proxy_file,
            #     '-c:v', 'libvpx-vp9',
            #     '-filter_complex', f'chromakey={background_color}:0.01:0.01,format=yuva420p[output]',
            #     '-map', '[output]',
            #     '-pix_fmt', 'yuva420p',
            #     '-metadata:s:v:0', 'alpha_mode=1',
            #     output_file
            # ]

            subprocess.run(cmd, check=True)
            created_files.append(output_file)

    return created_files

async def add_to_existing_pack(bot, user_id, name, emoji_files):
    try:
        await bot.initialize()
        # Upload and add the rest of the stickers
        for emoji_file in emoji_files[1:]:
            await asyncio.sleep(0.2)
            with open(emoji_file, 'rb') as sticker_file:
                uploaded_sticker = await bot.upload_sticker_file(
                    user_id=user_id,
                    sticker_format='video',
                    sticker=sticker_file
                )
                file_id = uploaded_sticker.file_id

            await bot.add_sticker_to_set(
                user_id=user_id,
                name=f"{name}",
                sticker=InputSticker(
                    sticker=file_id,
                    emoji_list=["🎥"],
                    format='video'
                )
            )
        await bot.send_message(chat_id=user_id, text=f"Набор стикеров для эмодзи создан и доступен по ссылке:\nhttps://t.me/addemoji/{name}")
    except Exception as e:
        print(f"Ошибка при добавлении стикера в пак: {e}")
        return None

async def create_custom_emoji_set(bot, user_id, name, title, emoji_files):
    await bot.initialize()
    try:
        # Upload the first sticker file and get the file_id
        with open(emoji_files[0], 'rb') as sticker_file:
            uploaded_sticker = await bot.upload_sticker_file(
                user_id=user_id,
                sticker_format="video",
                sticker=sticker_file
            )
            file_id = uploaded_sticker.file_id
            print(f"Загружен файл стикера: {file_id}")
    except Exception as e:
        print(f"Ошибка при создании cтикера: {e}")
        return None

    try:
        # Create the sticker set using the file_id
        await bot.create_new_sticker_set(
            user_id=user_id,
            name=f"{name}_by_{bot.username}",
            title=title,
            stickers=[
                InputSticker(
                    sticker=file_id,
                    emoji_list=["🎥"],
                    format='video',
                )
            ],
            sticker_type=Sticker.CUSTOM_EMOJI
        )
    except Exception as e:
        print(f"Ошибка при создании набора стикеров: {e}")
        return None

    try:
        # Upload and add the rest of the stickers
        for emoji_file in emoji_files[1:]:
            await asyncio.sleep(0.1)
            with open(emoji_file, 'rb') as sticker_file:
                uploaded_sticker = await bot.upload_sticker_file(
                    user_id=user_id,
                    sticker_format='video',
                    sticker=sticker_file
                )
                file_id = uploaded_sticker.file_id

            await bot.add_sticker_to_set(
                user_id=user_id,
                name=f"{name}_by_{bot.username}",
                sticker=InputSticker(
                    sticker=file_id,
                    emoji_list=["🎥"],
                    format='video'
                )
            )
        await bot.send_message(chat_id=user_id, text=f"Набор стикеров для эмодзи создан и доступен по ссылке:\nhttps://t.me/addemoji/{name}_by_{bot.username}")
    except Exception as e:
        print(f"Ошибка при создании добавлении стикера в пак: {e}")

    return f"https://t.me/addemoji/{name}_by_{bot.username}"



async def main(input_video, name="", user_id=548414066):
    output_dir = "/Users/admin/Downloads/emojikc"


    # Очищаем директорию перед началом работы
    clear_directory(output_dir)

    if not os.path.exists(output_dir):
        os.makedirs(output_dir)


    ## --------------------- BACKGROUND COLOR ---------------------
    # background_color="0x000000"
    created_files = process_video(input_video, output_dir)
    print("Обработка видео завершена.")

    # Обновленные значения
    # bot_token = "7445477091:AAGOqZ_0_5vTkhHRNfK2iHWgk4ejM8UkL_8"
    bot_token = "7486051673:AAEg2bzMqec1NkFK8tHycLn8gvGxK6xQ6ww"
    sticker_set_title = "@elysium_fm expirements"

    bot = Bot(token=bot_token)

    if name == "":
        name = "elysium_expiremeents"  + str(int(time.time()))
        emoji_set_url = await create_custom_emoji_set(bot, user_id, name, sticker_set_title, created_files)
    else:
        emoji_set_url = await add_to_existing_pack(bot, user_id, name, created_files)

    print(f"Ссылка на набор эмодзи: {emoji_set_url}")
    await bot.shutdown()

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Использование: python3 newemoji.py input.mp4")
        sys.exit(1)

    input_video = sys.argv[1]
    set_name = ""
    user_id = 0
    try:
        set_name = sys.argv[2]
    except:
        # -------------------- ИМЯ НАБОРА СТИКЕРОВ -------------------
        set_name = ""

    try:
        user_id = int(sys.argv[3])
    except:
        user_id = 548414066

    asyncio.run(main(input_video, name=set_name, user_id=user_id))
