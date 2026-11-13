from __future__ import annotations

import os
import re
import subprocess
from pathlib import Path

from docx import Document
from docx.enum.section import WD_SECTION
from docx.enum.table import WD_TABLE_ALIGNMENT
from docx.enum.text import WD_ALIGN_PARAGRAPH, WD_BREAK
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Cm, Pt, RGBColor

ROOT = Path(__file__).resolve().parents[2]
IMG = ROOT / "docs/manuals/images/word"
OUT = ROOT / "docs/manuals"
DEPLOY_MD = ROOT / "deploy/centos7-sqlite/manual/部署手册.md"
DEPLOY_DIR = ROOT / "deploy/centos7-sqlite/manual"

FONT = "Microsoft YaHei"
MONO = "Consolas"
BLUE = RGBColor(31, 78, 121)
GRAY = RGBColor(95, 99, 104)
LIGHT = "EAF1F8"


def set_run_font(run, name=FONT, size=10.5, bold=None, color=None):
    run.font.name = name
    run._element.rPr.rFonts.set(qn("w:eastAsia"), name)
    run.font.size = Pt(size)
    if bold is not None:
        run.bold = bold
    if color is not None:
        run.font.color.rgb = color


def shade(cell, fill):
    tc_pr = cell._tc.get_or_add_tcPr()
    shd = OxmlElement("w:shd")
    shd.set(qn("w:fill"), fill)
    tc_pr.append(shd)


def set_cell_margins(table, top=80, bottom=80, left=120, right=120):
    tbl_pr = table._tbl.tblPr
    mar = OxmlElement("w:tblCellMar")
    for side, value in (("top", top), ("bottom", bottom), ("left", left), ("right", right)):
        node = OxmlElement(f"w:{side}")
        node.set(qn("w:w"), str(value))
        node.set(qn("w:type"), "dxa")
        mar.append(node)
    tbl_pr.append(mar)


def page_number_footer(section, label):
    footer = section.footer
    footer.is_linked_to_previous = False
    p = footer.paragraphs[0]
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    r = p.add_run(label + "    第 ")
    set_run_font(r, size=8.5, color=GRAY)
    fld_begin = OxmlElement("w:fldChar")
    fld_begin.set(qn("w:fldCharType"), "begin")
    instr = OxmlElement("w:instrText")
    instr.set(qn("xml:space"), "preserve")
    instr.text = " PAGE "
    fld_end = OxmlElement("w:fldChar")
    fld_end.set(qn("w:fldCharType"), "end")
    r2 = p.add_run()
    r2._r.append(fld_begin)
    r2._r.append(instr)
    r2._r.append(fld_end)
    set_run_font(r2, size=8.5, color=GRAY)
    r3 = p.add_run(" 页")
    set_run_font(r3, size=8.5, color=GRAY)


def header_text(section, text):
    p = section.header.paragraphs[0]
    p.text = ""
    p.alignment = WD_ALIGN_PARAGRAPH.RIGHT
    r = p.add_run(text)
    set_run_font(r, size=8.5, color=GRAY)


def setup_document(title, subject):
    doc = Document()
    section = doc.sections[0]
    section.page_width = Cm(21)
    section.page_height = Cm(29.7)
    section.top_margin = Cm(2.2)
    section.bottom_margin = Cm(2.0)
    section.left_margin = Cm(2.2)
    section.right_margin = Cm(2.2)

    styles = doc.styles
    normal = styles["Normal"]
    normal.font.name = FONT
    normal._element.rPr.rFonts.set(qn("w:eastAsia"), FONT)
    normal.font.size = Pt(10.5)
    normal.paragraph_format.line_spacing = 1.35
    normal.paragraph_format.space_after = Pt(5)

    for name, size, color in (("Heading 1", 17, BLUE), ("Heading 2", 13.5, BLUE), ("Heading 3", 11.5, RGBColor(54, 95, 145))):
        st = styles[name]
        st.font.name = FONT
        st._element.rPr.rFonts.set(qn("w:eastAsia"), FONT)
        st.font.size = Pt(size)
        st.font.bold = True
        st.font.color.rgb = color
        st.paragraph_format.space_before = Pt(14)
        st.paragraph_format.space_after = Pt(7)

    doc.core_properties.title = title
    doc.core_properties.subject = subject
    doc.core_properties.author = "bbs-go 平台"
    return doc


def cover(doc, title, subtitle, tagline, meta_rows):
    for _ in range(3):
        doc.add_paragraph()
    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    r = p.add_run(title)
    set_run_font(r, size=30, bold=True, color=BLUE)
    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    r = p.add_run(subtitle)
    set_run_font(r, size=16, color=RGBColor(54, 95, 145))
    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    r = p.add_run(tagline)
    set_run_font(r, size=10.5, color=GRAY)

    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    r = p.add_run("—" * 26)
    set_run_font(r, size=10, color=RGBColor(180, 190, 200))

    table = doc.add_table(rows=len(meta_rows), cols=2)
    table.alignment = WD_TABLE_ALIGNMENT.CENTER
    table.style = "Table Grid"
    set_cell_margins(table)
    for i, (k, v) in enumerate(meta_rows):
        table.rows[i].cells[0].width = Cm(4.2)
        table.rows[i].cells[1].width = Cm(8.8)
        shade(table.rows[i].cells[0], LIGHT)
        for j, text in enumerate((k, v)):
            cell = table.rows[i].cells[j]
            cell.text = ""
            para = cell.paragraphs[0]
            para.paragraph_format.space_after = Pt(2)
            run = para.add_run(text)
            set_run_font(run, size=9.5, bold=(j == 0), color=GRAY if j == 0 else None)
    doc.add_page_break()


def toc(doc, items):
    h = doc.add_paragraph()
    r = h.add_run("目  录")
    set_run_font(r, size=18, bold=True, color=BLUE)
    h.paragraph_format.space_after = Pt(10)
    for text in items:
        p = doc.add_paragraph()
        p.paragraph_format.left_indent = Cm(0.4)
        p.paragraph_format.space_after = Pt(4)
        r = p.add_run(text)
        set_run_font(r, size=10.5)
    doc.add_page_break()


def h1(doc, text):
    return doc.add_heading(text, level=1)


def h2(doc, text):
    return doc.add_heading(text, level=2)


def h3(doc, text):
    return doc.add_heading(text, level=3)


def p(doc, text, bold_prefix=None):
    para = doc.add_paragraph()
    if bold_prefix and text.startswith(bold_prefix):
        r = para.add_run(bold_prefix)
        set_run_font(r, bold=True)
        r = para.add_run(text[len(bold_prefix):])
        set_run_font(r)
    else:
        r = para.add_run(text)
        set_run_font(r)
    return para


def bullets(doc, items, numbered=False):
    for item in items:
        style = "List Number" if numbered else "List Bullet"
        para = doc.add_paragraph(style=style)
        para.paragraph_format.space_after = Pt(2)
        r = para.add_run(item)
        set_run_font(r, size=10)


def note(doc, title, text):
    table = doc.add_table(rows=1, cols=1)
    table.style = "Table Grid"
    set_cell_margins(table, 100, 100, 140, 140)
    cell = table.rows[0].cells[0]
    shade(cell, "FFF8E1")
    cell.text = ""
    para = cell.paragraphs[0]
    r = para.add_run(title)
    set_run_font(r, size=9.5, bold=True, color=RGBColor(150, 90, 0))
    para2 = cell.add_paragraph()
    r = para2.add_run(text)
    set_run_font(r, size=9.5)


def table(doc, headers, rows, widths=None):
    t = doc.add_table(rows=1, cols=len(headers))
    t.style = "Table Grid"
    t.alignment = WD_TABLE_ALIGNMENT.CENTER
    set_cell_margins(t)
    for i, h in enumerate(headers):
        cell = t.rows[0].cells[i]
        shade(cell, LIGHT)
        cell.text = ""
        para = cell.paragraphs[0]
        r = para.add_run(h)
        set_run_font(r, size=9.5, bold=True, color=BLUE)
    for row in rows:
        values = list(row) + [""] * max(0, len(headers) - len(row))
        cells = t.add_row().cells
        for i, value in enumerate(values[:len(headers)]):
            cells[i].text = ""
            para = cells[i].paragraphs[0]
            para.paragraph_format.space_after = Pt(1)
            r = para.add_run(str(value))
            set_run_font(r, size=9)
    if widths:
        for row in t.rows:
            for i, width in enumerate(widths):
                row.cells[i].width = Cm(width)
    doc.add_paragraph()


def image(doc, filename, caption, width=15.0):
    path = IMG / filename
    if not path.exists():
        raise FileNotFoundError(path)
    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p.paragraph_format.space_before = Pt(4)
    p.paragraph_format.space_after = Pt(2)
    run = p.add_run()
    run.add_picture(str(path), width=Cm(width))
    cap = doc.add_paragraph()
    cap.alignment = WD_ALIGN_PARAGRAPH.CENTER
    cap.paragraph_format.space_after = Pt(8)
    r = cap.add_run(caption)
    set_run_font(r, size=8.5, color=GRAY)


def code(doc, text):
    para = doc.add_paragraph()
    para.paragraph_format.left_indent = Cm(0.3)
    para.paragraph_format.space_before = Pt(3)
    para.paragraph_format.space_after = Pt(6)
    pPr = para._p.get_or_add_pPr()
    shd = OxmlElement("w:shd")
    shd.set(qn("w:fill"), "F4F6F8")
    pPr.append(shd)
    r = para.add_run(text)
    set_run_font(r, name=MONO, size=8.5)


def add_page_break(doc):
    doc.add_page_break()


def sanitize_admin_brand():
    """把后台截图左上角的产品名替换为平台名称。"""
    from PIL import Image, ImageDraw, ImageFont
    font_path = "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc"
    try:
        font = ImageFont.truetype(font_path, 11, index=0)
    except OSError:
        font = ImageFont.load_default()
    for path in sorted(IMG.glob("admin-*.png")):
        im = Image.open(path).convert("RGB")
        if im.width < 1100:
            continue
        draw = ImageDraw.Draw(im)
        draw.rectangle((44, 8, 185, 31), fill=(248, 250, 252))
        draw.text((51, 11), "平台知识管理工具", font=font, fill=(31, 78, 121))
        im.save(path, optimize=True)


# ---------------- 用户手册 ----------------
def build_user_manual():
    doc = setup_document("平台知识管理工具用户手册", "平台知识管理工具使用说明")
    doc.core_properties.author = "平台知识管理工具"
    header_text(doc.sections[0], "平台知识管理工具用户手册")
    page_number_footer(doc.sections[0], "平台知识管理工具用户手册")
    cover(
        doc,
        "平台知识管理工具",
        "用户手册",
        "问答 · 知识库 · 搜索 · 个人中心",
        [("文档版本", "V1.0"), ("适用系统", "平台知识管理工具"), ("编写日期", "2026-11-13"), ("示例环境", "当前 Docker 部署实例")],
    )
    toc(doc, [
        "1  系统概览",
        "2  登录与账号",
        "3  首页与导航",
        "4  浏览问答",
        "5  搜索问题与知识",
        "6  发布问题",
        "7  回答、评论与采纳",
        "8  知识库",
        "9  个人中心",
        "10  举报与使用规范",
        "11  常见问题",
    ])

    h1(doc, "1  系统概览")
    p(doc, "本系统是一个面向平台问题处理与知识沉淀的知识管理工具。用户可以在“问答”中提交问题、跟踪处理状态，在“知识库”中查看标准答案、排查经验和操作说明。")
    table(doc, ["一级入口", "用途", "典型操作"], [
        ["问答", "问题清单与讨论", "浏览问题、提问、回答、评论、采纳答案"],
        ["知识库", "可复用的知识条目", "浏览文章、查看标准答案与操作说明、评论"],
        ["搜索", "跨问答和知识库检索", "按关键词、分类、时间筛选"],
        ["+ 发表", "创建内容", "发帖、发动态、提问"],
        ["个人中心", "账号与资料", "查看个人资料、设置邮箱/密码、绑定第三方账号"],
    ], widths=[3.2, 5.5, 7.0])
    note(doc, "说明", "不同账号的可见菜单和操作权限可能不同；如果页面没有某个入口，通常表示当前账号没有对应权限。")

    h1(doc, "2  登录与账号")
    h2(doc, "2.1  登录")
    bullets(doc, [
        "打开管理员提供的访问地址，例如 http://服务器IP:端口/。",
        "在登录页选择“账号登录”，输入用户名或邮箱、密码。",
        "按页面提示完成验证码，然后点击“登录”。",
        "登录成功后进入首页；点击右上角头像可以进入个人中心或退出登录。",
    ], numbered=True)
    h2(doc, "2.2  注册与账号开通")
    p(doc, "如果系统开放注册，可点击“没有账号？点击这里去注册”完成注册；如果注册入口不可见，说明管理员关闭了开放注册或当前部署由管理员统一创建账号，请联系管理员开通。")
    h2(doc, "2.3  忘记密码")
    p(doc, "登录页不提供自助邮箱重置。点击“联系管理员”，按弹窗提示联系系统管理员，由管理员在后台用户列表中重置密码。")
    image(doc, "user-contact-admin.png", "图 2-1  登录页“联系管理员”弹窗", 13.5)
    note(doc, "安全提示", "不要通过聊天工具发送明文密码；管理员重置密码后，请首次登录后尽快修改为自己的密码。")

    h1(doc, "3  首页与导航")
    p(doc, "首页顶部提供“问答”“知识库”和搜索框，右侧“+ 发表”用于创建内容。左侧可按“最新”“推荐”或具体分类浏览；中间卡片展示问题标题、摘要、分类、状态、点赞数和评论数。")
    image(doc, "user-home.png", "图 3-1  首页：问题列表、分类和搜索入口", 15.0)
    bullets(doc, [
        "最新：按发布时间排序，适合查看最近提交的问题。",
        "推荐：查看管理员推荐或重点内容。",
        "分类：按问题所属分类筛选；分类名称由管理员维护。",
        "卡片右下角分别显示点赞数和评论数。",
    ])

    h1(doc, "4  浏览问答")
    h2(doc, "4.1  问答列表")
    p(doc, "点击顶部“问答”进入问答列表。列表支持“最新发布/最新回复”切换和分类筛选，卡片上的状态标识表示问题是否已解决。")
    image(doc, "user-topics.png", "图 4-1  问答列表与分类筛选", 15.0)
    h2(doc, "4.2  问题详情")
    p(doc, "点击问题标题进入详情。详情页通常包含：问题标题、分类、状态、正文、平台模块/业务场景/来源/优先级/严重程度/责任人等业务字段，以及回答、评论和点赞区域。")
    image(doc, "user-topic-detail.png", "图 4-2  问题详情页（节选）", 14.5)
    bullets(doc, [
        "状态：待解决、已解决等，由问题作者或管理员维护。",
        "业务字段：帮助管理员按平台模块、业务场景、优先级等维度筛选和跟踪。",
        "回答/评论：在页面底部输入内容后提交；回复某条评论时使用该评论下的“回复”。",
        "采纳：问题作者可以对合适的回答执行“采纳答案”；采纳后可取消。",
        "点赞：对问题或回答点赞。",
        "举报：对不合适的内容提交举报，由管理员处理。",
    ])

    h1(doc, "5  搜索问题与知识")
    p(doc, "在顶部搜索框输入关键词并回车，进入搜索结果页。可以切换“全部/问答/知识库”，并按分类和时间范围筛选。")
    image(doc, "user-search.png", "图 5-1  搜索结果页", 15.0)
    bullets(doc, [
        "搜索会匹配问题标题、正文和知识库文章内容。",
        "如果没有结果，尝试减少关键词、切换内容类型或放宽时间范围。",
        "搜索索引由管理员维护；大量内容变更后如结果异常，可联系管理员重建索引。",
    ])

    h1(doc, "6  发布问题")
    p(doc, "点击右上角“+ 发表”，选择“提问”。提问前必须由管理员配置至少一个“问答分类”；如果页面提示“暂无可用问答分类”，请联系管理员把分类类型设置为“问答分类”。")
    image(doc, "user-topic-create.png", "图 6-1  提问编辑页", 15.0)
    h2(doc, "6.1  填写步骤")
    bullets(doc, [
        "选择分类：只能选择类型为“问答分类”的分类；子分类会自动继承父分类类型。",
        "填写标题：用一句话概括问题，避免只写“求助”“有问题”。",
        "填写正文：描述现象、影响范围、复现步骤、已尝试的操作和期望结果。",
        "补充业务字段：按需填写平台模块、业务场景、问题来源、优先级、严重程度、责任人/处理组。",
        "可选：添加附件、图片、投票。附件和图片受管理员配置的类型、大小和数量限制。",
        "点击“发布问题”提交；提交后可在问题详情继续补充评论。",
    ], numbered=True)
    h2(doc, "6.2  提问建议")
    bullets(doc, [
        "标题包含具体对象和现象，例如“仿真任务在 80% 后卡住”。",
        "正文提供环境、版本、时间、日志或截图，便于他人定位。",
        "不要在正文粘贴密码、密钥、个人隐私等敏感信息。",
    ])

    h1(doc, "7  回答、评论与采纳")
    h2(doc, "7.1  发表回答/评论")
    p(doc, "在问题详情底部输入框填写内容，点击提交即可发表一级评论。要回复某条评论，点击该评论下的“回复”，输入后提交；回复会显示在父评论下方。")
    h2(doc, "7.2  采纳答案")
    p(doc, "问题作者可以在合适的回答上点击“采纳答案”。采纳后该回答会置顶或标记为已采纳，问题状态显示为已解决；再次点击可取消采纳。")
    h2(doc, "7.3  删除与举报")
    bullets(doc, [
        "自己发表的评论可以删除；管理员或有权限的用户也可以删除违规评论。",
        "删除评论后，问题/文章的评论数量会同步减少。",
        "对不适合的内容使用“举报”，选择原因并提交；管理员会在后台处理。",
    ])

    h1(doc, "8  知识库")
    p(doc, "点击顶部“知识库”进入文章列表。知识库用于沉淀标准答案、排查经验、操作说明等可复用内容；点击文章标题查看详情，支持评论和举报。")
    image(doc, "user-articles.png", "图 8-1  知识库列表", 15.0)
    image(doc, "user-article-detail.png", "图 8-2  知识库文章详情（节选）", 13.5)
    note(doc, "提示", "文章底部会显示浏览量、评论数和点赞数；评论数由系统根据有效评论实时维护。")

    h1(doc, "9  个人中心")
    h2(doc, "9.1  个人资料")
    p(doc, "点击右上角头像，选择“个人中心”，可以查看昵称、用户名、个人简介、主页、头像等信息。")
    image(doc, "user-profile.png", "图 9-1  个人中心", 14.5)
    h2(doc, "9.2  账号设置")
    p(doc, "在“账号设置”中可以设置/修改邮箱、密码，绑定或解绑微信、Google、GitHub 等第三方账号（具体可用项由管理员配置）。")
    image(doc, "user-account.png", "图 9-2  账号设置", 14.5)
    bullets(doc, [
        "邮箱：用于账号识别和通知；请填写真实可用的邮箱。",
        "密码：设置后可使用账号密码登录；忘记密码请联系管理员重置。",
        "第三方绑定：按页面提示完成授权；解绑前确认仍能通过其他方式登录。",
    ])

    h1(doc, "10  举报与使用规范")
    bullets(doc, [
        "不发布违法、侵权、广告、恶意攻击或涉及他人隐私的内容。",
        "提问前先搜索是否已有相同问题；重复问题会被合并或删除。",
        "回答尽量给出可执行的步骤、命令或判断依据。",
        "发现违规内容使用“举报”，不要以辱骂等方式自行处理。",
        "账号仅限本人使用，不要共享密码或验证码。",
    ])

    h1(doc, "11  常见问题")
    table(doc, ["问题", "处理方式"], [
        ["登录失败", "确认用户名/邮箱和密码；验证码是否过期；多次失败可联系管理员重置密码。"],
        ["看不到“提问”或提示暂无问答分类", "联系管理员在后台分类管理中创建/修改“问答分类”。"],
        ["评论删除后列表数量未变", "系统会在删除时同步减少计数；如仍异常，刷新页面或联系管理员。"],
        ["知识库文章评论数显示 0", "确认是否已有有效评论；如仍异常，联系管理员检查评论数据。"],
        ["图片/附件上传失败", "检查文件类型、大小、数量限制；仍失败请联系管理员。"],
        ["账号被禁用/无法访问", "联系管理员确认账号状态和角色权限。"],
        ["想修改用户名", "用户名通常只能设置一次；在账号设置中谨慎操作，或联系管理员。"],
    ], widths=[5.2, 10.0])
    p(doc, "如果以上方式仍不能解决，请通过团队内部渠道联系系统管理员，并提供账号、页面地址、操作时间和错误截图。")

    out = OUT / "平台知识管理工具用户手册.docx"
    doc.save(out)
    print("saved", out)


# ---------------- 管理员手册 ----------------
def build_admin_manual():
    doc = setup_document("平台知识管理工具管理员手册", "平台知识管理工具后台管理说明")
    doc.core_properties.author = "平台知识管理工具"
    header_text(doc.sections[0], "平台知识管理工具管理员手册")
    page_number_footer(doc.sections[0], "平台知识管理工具管理员手册")
    cover(
        doc,
        "平台知识管理工具",
        "管理员手册",
        "分类 · 用户 · 角色权限 · 内容治理 · 系统设置",
        [("文档版本", "V1.0"), ("适用系统", "平台知识管理工具"), ("编写日期", "2026-11-13"), ("示例环境", "当前 Docker 部署实例")],
    )
    toc(doc, [
        "1  后台入口与权限模型",
        "2  工作台",
        "3  分类管理",
        "4  用户管理",
        "5  角色与权限",
        "6  问题库管理",
        "7  知识库管理",
        "8  举报与违禁词",
        "9  系统设置",
        "10  搜索索引与 Sitemap",
        "11  邮件日志",
        "12  运维与备份",
        "13  安全建议",
        "14  常见问题",
    ])

    h1(doc, "1  后台入口与权限模型")
    p(doc, "后台入口为 http://服务器IP:端口/dashboard。使用管理员账号登录前台后，点击右上角头像进入“管理后台”，或直接访问后台地址。")
    bullets(doc, [
        "系统管理员（owner）：拥有全部后台权限，可管理用户、角色、内容与系统设置。",
        "自定义角色：由 owner 创建并分配权限；只能看到被授权的菜单和操作。",
        "非 owner 管理员不能分配 owner 或系统角色，避免越权。",
        "后台所有操作以当前登录账号身份执行；请勿共享管理员账号。",
    ])
    note(doc, "示例地址", "本文截图来自当前 Docker 部署实例。实际部署时请把地址替换为你的服务器地址和端口。")

    h1(doc, "2  工作台")
    p(doc, "登录后台默认进入“工作台”，展示问题总数、知识条目、已解决问题、待处理举报、待审核文章、邮件失败等指标，以及最近问题列表和快捷入口。")
    image(doc, "admin-dashboard.png", "图 2-1  管理工作台", 15.0)
    bullets(doc, [
        "左侧导航按“平台 / 知识支撑 / 用户支持 / 系统”分组。",
        "快捷入口只显示当前账号有权限访问的功能。",
        "指标卡片和待处理事项可点击进入对应列表。",
        "右上角可返回前台首页、切换主题和退出登录。",
    ])

    h1(doc, "3  分类管理")
    p(doc, "进入“知识支撑 → 分类”。分类既决定前台导航，也决定内容类型：普通分类用于帖子/动态，问答分类用于“提问”。这是问答发布的前置条件。")
    image(doc, "admin-categories.png", "图 3-1  分类列表：类型列区分普通分类与问答分类", 15.0)
    h2(doc, "3.1  类型规则")
    table(doc, ["类型", "用途", "前台表现"], [
        ["普通分类（帖子/动态）", "普通帖子、动态", "不会出现在“提问”分类选择器中"],
        ["问答分类", "提问/问题跟踪", "出现在“提问”页面；没有问答分类时无法发布问题"],
    ], widths=[4.5, 5.0, 6.0])
    bullets(doc, [
        "只有一级分类可以选择类型；子分类会自动继承父分类类型。",
        "修改一级分类类型时，系统会联动更新其子分类。",
        "如果用户反馈“暂无可用问答分类”，优先检查是否把分类类型设置成了“普通分类”。",
        "分类描述可以不填；名称建议简短、可识别。",
    ])
    h2(doc, "3.2  新建/编辑分类")
    image(doc, "admin-category-form.png", "图 3-2  新建分类：类型必须选择正确", 12.5)
    bullets(doc, [
        "点击“新建”，选择父级分类（可选）。",
        "一级分类必须选择类型：普通分类或问答分类。",
        "填写名称、描述，按需上传图标，选择状态。",
        "保存后可在列表中调整排序、编辑或删除。",
    ], numbered=True)
    note(doc, "关键检查", "如果“提问”页提示没有问答分类，请打开分类列表，确认至少有一个一级分类的类型是“问答分类”，且状态为正常。")

    h1(doc, "4  用户管理")
    p(doc, "进入“用户支持 → 用户列表”。支持按用户名、昵称、邮箱等条件搜索，查看用户状态、角色、注册时间，并进行编辑、重置密码、禁言/解禁等操作。")
    image(doc, "admin-users.png", "图 4-1  用户列表", 15.0)
    h2(doc, "4.1  创建单个用户")
    image(doc, "admin-user-create.png", "图 4-2  新建用户对话框", 14.0)
    bullets(doc, [
        "用户名：必填，5–12 位，仅英文字母、数字、下划线，且以字母开头。",
        "邮箱：必填，需唯一。",
        "昵称：必填。",
        "密码：仅创建时显示，必填；编辑用户时不会回显密码。",
        "角色：可选；非 owner 只能分配自己有权限分配的角色。",
    ], numbered=False)
    note(doc, "密码安全", "创建后请通过安全渠道把初始密码告知用户，并要求其尽快修改；不要在群聊中长期保留明文密码。")
    h2(doc, "4.2  Excel 批量导入")
    p(doc, "点击用户列表右上角“批量导入”。先下载模板，按表头填写后上传；单行失败不会影响其他行。")
    image(doc, "admin-user-import.png", "图 4-3  批量导入用户", 14.0)
    table(doc, ["列", "是否必填", "说明"], [
        ["用户名", "是", "5–12 位字母/数字/下划线，字母开头"],
        ["邮箱", "是", "唯一，不能与已有账号重复"],
        ["昵称", "是", "显示名称"],
        ["密码", "否", "为空时使用导入页填写的默认密码；行内密码优先"],
    ], widths=[3.2, 2.6, 9.5])
    bullets(doc, [
        "仅支持 .xlsx，文件不超过 5MB，单次最多 1000 行。",
        "导入页可选择统一角色；所有导入用户使用该角色，不选则不分配。",
        "默认密码可选；行内有密码时优先使用行内密码。",
        "导入完成后展示成功账号、角色分配失败、失败账号及原因；失败行号对应 Excel 实际行号。",
        "默认密码至少 6 位、最多 72 字节；建议使用强密码。",
    ], numbered=True)
    h2(doc, "4.3  重置密码与禁用")
    p(doc, "在用户列表操作列选择“重置密码”，系统会生成一个新密码并仅展示一次；复制后交给用户。需要限制登录时，可对账号执行禁言/禁用操作。")

    h1(doc, "5  角色与权限")
    p(doc, "进入“用户支持 → 角色”。角色用于控制后台菜单和操作权限；系统角色由系统维护，自定义角色可按需分配权限。")
    image(doc, "admin-roles.png", "图 5-1  角色与权限分配", 15.0)
    bullets(doc, [
        "owner（系统管理员）不受权限点限制，拥有全部后台能力。",
        "系统角色（如 owner）不应随意分配；非 owner 不能把 owner/系统角色授予他人。",
        "创建用户、编辑用户、重置密码、分类管理、内容审核等均为独立权限点，可按最小权限原则分配。",
        "删除或调整角色前，确认没有账号依赖该角色。",
    ])

    h1(doc, "6  问题库管理")
    p(doc, "进入“知识支撑 → 问题库”。可按标题、分类、状态、类型等筛选问题，查看详情，进行编辑、审核、推荐、置顶、删除或修改处理状态。")
    image(doc, "admin-topics.png", "图 6-1  问题库管理", 15.0)
    bullets(doc, [
        "待审核内容会出现在工作台和列表中，审核通过后才对普通用户可见。",
        "推荐/置顶会影响前台展示，请按运营规则使用。",
        "处理状态包括待处理、处理中、已解决、已关闭、已归档；已解决可记录解决时间。",
        "删除内容为软删除，历史记录仍保留在数据库中。",
    ])

    h1(doc, "7  知识库管理")
    p(doc, "进入“知识支撑 → 知识库管理”。用于审核、编辑和维护知识库文章；文章评论数、浏览量和点赞数会在列表/详情展示。")
    image(doc, "admin-articles.png", "图 7-1  知识库管理", 15.0)
    bullets(doc, [
        "文章可设置待审核；审核通过后前台可见。",
        "文章评论使用与问答相同的评论体系；删除评论会同步回退评论数。",
        "知识库适合沉淀标准答案、排查经验、操作说明，不建议作为问题讨论区。",
    ])

    h1(doc, "8  举报与违禁词")
    h2(doc, "8.1  用户举报")
    p(doc, "进入“用户支持 → 用户举报”。查看举报对象、原因和状态，进入目标内容核实后标记“已处理”或“忽略”。")
    image(doc, "admin-user-reports.png", "图 8-1  用户举报列表", 15.0)
    h2(doc, "8.2  违禁词")
    p(doc, "进入“平台 → 违禁词”，维护词条或正则规则，用于发帖/评论内容过滤。规则变更后对新提交内容生效。")
    image(doc, "admin-forbidden-words.png", "图 8-2  违禁词管理", 15.0)

    h1(doc, "9  系统设置")
    p(doc, "进入“系统 → 设置”。设置项较多，保存时按分区提交；修改登录、注册、SMTP 等配置会影响所有用户，请谨慎操作。")
    image(doc, "admin-settings.png", "图 9-1  系统设置", 15.0)
    table(doc, ["分区", "主要配置", "说明"], [
        ["通用", "站点名称、Logo、描述、URL、关键词、公告", "站点 URL 影响邮件、sitemap 和绝对链接"],
        ["内容", "模块开关、默认分类、推荐标签、附件、评论可见", "控制问答/知识库/动态等模块"],
        ["反垃圾", "发帖验证码、邮箱验证、审核、观察期", "按社区风险调整"],
        ["通知", "SMTP、邮件通知间隔、邮箱白名单、通知类型", "SMTP 未配置时邮件发送会失败"],
        ["登录", "开放注册、密码/短信/微信/Google/GitHub 登录", "关闭注册后仅管理员可创建账号"],
        ["上传", "本地上传/OSS/COS/S3、图片样式", "切换存储前先验证凭证和访问域名"],
        ["页面", "关于页、底部链接", "支持中英文内容"],
        ["脚本", "全站 head 注入脚本", "仅配置可信代码，避免 XSS"],
        ["搜索索引/Sitemap", "重建索引、生成 sitemap", "内容批量调整后使用"],
    ], widths=[2.6, 7.2, 6.0])
    note(doc, "SMTP 提醒", "当前部署默认未配置 SMTP。邮件通知和邮件相关能力需要先在“通知”中填写 SMTP 并测试；前台已不提供自助邮箱找回密码。")
    h2(doc, "9.1  开放注册开关")
    p(doc, "在“登录”分区可关闭“开放注册”。关闭后，密码注册、短信首次建号、第三方首次建号都会被拦截；管理员在后台创建用户不受影响。")

    h1(doc, "10  搜索索引与 Sitemap")
    p(doc, "在系统设置中可手动重建搜索索引和生成 Sitemap。批量导入、删除内容或修改站点地址后，如搜索结果/站点地图异常，可执行一次重建。")
    bullets(doc, [
        "重建搜索索引会扫描现有帖子、文章、用户，期间搜索结果可能短暂不完整。",
        "生成 Sitemap 前确认站点 URL 已配置为公网可访问地址。",
        "索引文件位于数据目录的 topic_index；可在备份时一并保留，也可重建。",
    ])

    h1(doc, "11  邮件日志")
    p(doc, "进入“系统 → 邮件日志”，查看收件人、主题、业务类型、发送状态和错误信息，用于排查 SMTP 配置或通知发送失败。")
    image(doc, "admin-email-logs.png", "图 11-1  邮件日志", 15.0)

    h1(doc, "12  运维与备份")
    p(doc, "当前示例使用 Docker 部署。日常运维只需要保留数据目录、备份数据库和上传文件，并在升级前做一次完整备份。")
    bullets(doc, [
        "Docker 数据目录通常包含 data（配置、数据库/索引）、logs、uploads。",
        "备份前停止应用，避免 SQLite/MySQL 写入过程中复制出不一致文件。",
        "升级时先备份，再加载新镜像并重建容器；数据库迁移会在启动时自动执行。",
        "详细命令见部署手册或 docs/deployment.md。",
    ])

    h1(doc, "13  安全建议")
    bullets(doc, [
        "使用强密码并定期更换；不要共享 owner 账号。",
        "只给需要的管理员分配最小权限；定期复核角色成员。",
        "生产环境使用 HTTPS，限制后台访问来源 IP。",
        "SMTP 授权码、OAuth Secret 等敏感信息不要写入文档或代码仓库。",
        "定期备份并验证恢复流程；保留至少一份离线备份。",
        "及时清理离职人员账号和不再使用的角色。",
    ])

    h1(doc, "14  常见问题")
    table(doc, ["问题", "排查/处理"], [
        ["用户提问时提示暂无问答分类", "到“分类”确认存在一级分类且类型为“问答分类”。"],
        ["批量导入全部失败", "检查表头是否为用户名/邮箱/昵称/密码、文件是否为 .xlsx、用户名格式、邮箱是否重复。"],
        ["导入结果行号与 Excel 不一致", "以结果页显示的实际 Excel 行号为准；空行/隐藏行可能导致视觉偏差。"],
        ["SMTP 测试失败", "检查 Host/Port/账号/授权码/SSL、发件人限制和网络连通性，查看邮件日志错误。"],
        ["文章评论数显示 0", "确认有有效评论；系统已修复文章评论计数，历史数据由迁移重算。"],
        ["删除评论后列表数量不变", "确认删除成功并刷新；系统会回退话题/文章/父评论/用户计数。"],
        ["无法访问后台", "确认账号拥有 dashboard 权限；owner 账号不受权限点限制。"],
        ["升级后页面异常", "查看容器日志，确认数据目录挂载正确、迁移执行成功；必要时回滚镜像和数据库备份。"],
    ], widths=[5.3, 10.0])
    p(doc, "如需进一步排查，请提供账号、页面地址、操作时间、服务端日志和浏览器截图。")

    out = OUT / "平台知识管理工具管理员手册.docx"
    doc.save(out)
    print("saved", out)

# ---------------- 部署手册（Markdown -> DOCX，再转 PDF） ----------------
def clean_inline(text):
    text = re.sub(r"\[([^\]]+)\]\([^)]+\)", r"\1", text)
    text = text.replace("**", "").replace("`", "")
    return text.strip()


def build_deployment_docx():
    lines = DEPLOY_MD.read_text(encoding="utf-8").splitlines()
    doc = setup_document("bbs-go CentOS 7 离线部署手册（SQLite 版）", "CentOS 7 离线 Docker 部署")
    header_text(doc.sections[0], "bbs-go CentOS 7 离线部署手册")
    page_number_footer(doc.sections[0], "部署手册")
    cover(
        doc,
        "bbs-go 离线部署手册",
        "CentOS 7 · Docker · SQLite",
        "单进程 · 无 MySQL · 无 Node SSR",
        [("文档版本", "V1.0"), ("适用系统", "CentOS 7.x x86_64"), ("数据库", "SQLite"), ("编写日期", "2026-11-13")],
    )

    i = 0
    while i < len(lines):
        line = lines[i].rstrip()
        stripped = line.strip()
        if stripped.startswith("```"):
            i += 1
            buf = []
            while i < len(lines) and not lines[i].strip().startswith("```"):
                buf.append(lines[i])
                i += 1
            i += 1
            code(doc, "\n".join(buf))
            continue
        if stripped.startswith("|") and i + 1 < len(lines) and re.match(r"^\|[\s:|-]+\|$", lines[i + 1].strip()):
            headers = [clean_inline(x) for x in stripped.strip("|").split("|")]
            rows = []
            i += 2
            while i < len(lines) and lines[i].strip().startswith("|"):
                rows.append([clean_inline(x) for x in lines[i].strip().strip("|").split("|")])
                i += 1
            table(doc, headers, rows)
            continue
        if stripped.startswith("- [ ] "):
            bullets(doc, [f"未完成：{clean_inline(stripped[6:])}"])
        elif stripped.startswith("- ") or stripped.startswith("* "):
            items = []
            while i < len(lines) and (lines[i].strip().startswith("- ") or lines[i].strip().startswith("* ")):
                items.append(clean_inline(lines[i].strip()[2:]))
                i += 1
            bullets(doc, items)
            continue
        elif re.match(r"^\d+\.\s+", stripped):
            items = []
            while i < len(lines) and re.match(r"^\d+\.\s+", lines[i].strip()):
                items.append(clean_inline(re.sub(r"^\d+\.\s+", "", lines[i].strip())))
                i += 1
            bullets(doc, items, numbered=True)
            continue
        elif stripped.startswith("### "):
            h3(doc, clean_inline(stripped[4:]))
        elif stripped.startswith("## "):
            h2(doc, clean_inline(stripped[3:]))
        elif stripped.startswith("# "):
            if i > 0:
                h1(doc, clean_inline(stripped[2:]))
        elif stripped == "---":
            pass
        elif stripped:
            p(doc, clean_inline(stripped))
        i += 1

    out = DEPLOY_DIR / "部署手册.docx"
    doc.save(out)
    print("saved", out)
    # 生成 PDF 供离线包阅读
    profile = ROOT / ".devhome/lo-deploy"
    subprocess.run([
        "libreoffice", f"-env:UserInstallation=file://{profile}", "--headless",
        "--convert-to", "pdf", "--outdir", str(DEPLOY_DIR), str(out),
    ], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    print("saved", DEPLOY_DIR / "部署手册.pdf")


if __name__ == "__main__":
    sanitize_admin_brand()
    build_user_manual()
    build_admin_manual()
    build_deployment_docx()
