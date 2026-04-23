Repo-local public benchmark audio seed pack for the ingest benchmark.

Source dataset: `bond005/sberdevices_golos_10h_crowd`
Source page: https://huggingface.co/datasets/bond005/sberdevices_golos_10h_crowd

Included pinned clips:
- `voice-01.wav` -> `train:316` -> `джой можно на завтра записаться к окулисту`
- `voice-02.wav` -> `train:377` -> `закажи на дом молоко простоквашино два с половиной литра и жирностью один процент`
- `voice-03.wav` -> `train:923` -> `ряженка кубанская буренка четырехпроцентная литр`
- `voice-04.wav` -> `train:1501` -> `слушай алиса закажи мне один килограмм помидоров`
- `voice-05.wav` -> `train:3096` -> `закажи два пакета молока по одному литру один пакет кефира все с доставкой на дом`
- `voice-06.wav` -> `train:4533` -> `закажи мне хлеб кефир и яйца`
- `voice-07.wav` -> `train:11` -> `пожалуйста пирожные тирамису три штуки`
- `voice-08.wav` -> `train:249` -> `помидоры черри пол кило`
- `voice-09.wav` -> `train:309` -> `закажи пожалуйста чай тесс зеленый в пакетиках`
- `voice-10.wav` -> `train:312` -> `закажи пельмени сибирская коллекция домашние семьсот грамм`
- `voice-11.wav` -> `train:515` -> `молоко ильинское шестипроцентное один литр`
- `voice-12.wav` -> `train:697` -> `закажи колбасу вязанка`
- `voice-13.wav` -> `train:753` -> `я хочу заказать молоко домик в деревне емкостью один литр две штуки`
- `voice-14.wav` -> `train:1090` -> `добрый вечер мне нужны пельмени лукович килограмм со вкусом сыра вот привезите пожалуйста через полчаса`
- `voice-15.wav` -> `train:1294` -> `закажи мне мороженое белочка со вкусом клубники на дом`
- `voice-16.wav` -> `train:1775` -> `закажи молоко фирмы пармалат ультрапастеризованное жирность один и восемь процент один литр`
- `voice-17.wav` -> `train:1876` -> `алиса закажи бекон велком с доставкой на дом как можно быстрее`
- `voice-18.wav` -> `train:2313` -> `ассистент закажи воду святой источник негазированная с названием спорт на бутылке ноль семьдесят пять миллилитров`
- `voice-19.wav` -> `train:2445` -> `закажи мне пожалуйста из магазина монетка две булки черного уральского хлеба`
- `voice-20.wav` -> `train:2454` -> `купи мне пожалуйста молоко рогачевское ноль пять литров пяти процентной жирности`
- `voice-21.wav` -> `train:2612` -> `закажи пюре фрутоняня яблоко девяносто грамм стекло`
- `voice-22.wav` -> `train:2682` -> `купи молоко простоквашино один литр два с половиной процента жирности`
- `voice-23.wav` -> `train:2987` -> `закажи черный чай принцесса нури`
- `voice-24.wav` -> `train:3111` -> `закажи помидоры черри пятьсот грамм`
- `voice-25.wav` -> `train:3194` -> `алиса мне нужна вода родниковая объемом пять литров`
- `voice-26.wav` -> `train:3696` -> `закажи мне пожалуйста пельмени мираторг пятьсот грамм`
- `voice-27.wav` -> `train:2696` -> `алиса запиши пожалуйста меня на массаж на завтра на восемь вечера`
- `voice-28.wav` -> `train:3631` -> `помоги пожалуйста установить будильник завтра на утро`
- `voice-29.wav` -> `train:3312` -> `закажи пожалуйста моющее средство для мытья посуды фэри`
- `voice-30.wav` -> `train:3142` -> `с доставкой продукты с доставкой на дом килограмм яблок антоновка бананы два килограмма десяток яиц`

This directory is not the full voice benchmark corpus.

The canonical `voice` benchmark now contains `100` public-audio cases:

- `30` repo-local pinned clips from this directory
- `70` dataset-backed Golos rows that are fetched into the local benchmark cache during `-dataset-setup`

The repo-local pack is intentionally broader than the old smoke subset: it mixes
single-product grocery requests, quantity and delivery noise, branded products,
and explicit reject cases so the main `voice` benchmark starts from a realistic
seed before the dataset-backed cases extend scenario coverage further.

The corpus JSON remains the canonical source of truth for per-case SHA256, expected
outcomes, and benchmark metadata.
